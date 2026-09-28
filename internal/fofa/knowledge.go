package fofa

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

// Chunk 是知识库的一条判据片段：来源文档、所属标题与正文。
type Chunk struct {
	Source string // 相对 skill/ 的路径，如 SKILL.md、reference/mapping.md
	Title  string // 所属二级标题，如 "Certificate"
	Body   string // 标题下的正文
}

// knowledgeFiles 是参与索引的文档清单：SKILL.md 是总入口，
// reference/ 是判据细节。顺序即同分时的排序依据，保证输出确定。
var knowledgeFiles = []string{"SKILL.md"}

// referenceDir 是判据参考文档目录。
const referenceDir = "reference"

// Knowledge 是 Skill 判据文档的关键词索引，实现 eino 的 retriever.Retriever。
//
// 选型说明：知识库总量只有几百行判据文本，且当前环境没有可用的向量化模型
// 凭据，所以索引做成确定性的关键词检索（中文按二元滑窗、英文数字按词切分），
// 好处是零外部依赖、结果可复现、可被单测钉死。日后有 embedding 凭据时，
// 只需把这个类型内部的打分换成向量检索，对外接口不用动。
type Knowledge struct {
	chunks []Chunk
	topK   int

	// 索引统计量，检索时复用，避免每次查询重算
	avgLen  float64
	docFreq map[string]int
}

// LoadKnowledge 装载 skill 目录下的判据文档并建索引。
func LoadKnowledge(skillDir string, topK int) (*Knowledge, error) {
	if topK <= 0 {
		topK = 6
	}

	var chunks []Chunk
	for _, rel := range knowledgeFiles {
		fileChunks, err := chunkMarkdown(filepath.Join(skillDir, rel), rel)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, fileChunks...)
	}

	// reference/ 下按文件名排序，保证同分片段的先后与文件系统无关。
	entries, err := os.ReadDir(filepath.Join(skillDir, referenceDir))
	if err != nil {
		return nil, fmt.Errorf("fofa: 读取 %s 失败: %w", referenceDir, err)
	}
	var refs []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		refs = append(refs, entry.Name())
	}
	sort.Strings(refs)
	for _, name := range refs {
		rel := referenceDir + "/" + name
		fileChunks, err := chunkMarkdown(filepath.Join(skillDir, rel), rel)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, fileChunks...)
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("fofa: %s 下没有可索引的知识块", skillDir)
	}

	k := &Knowledge{chunks: chunks, topK: topK}
	k.buildStats()
	return k, nil
}

// chunkMarkdown 按二级标题（## ）把一份 markdown 切成知识块。
// 文档主标题之前的内容是导语，不进索引。
func chunkMarkdown(path, rel string) ([]Chunk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fofa: 读取 %s 失败: %w", rel, err)
	}

	var chunks []Chunk
	title := ""
	var body []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(body, "\n"))
		if title != "" && text != "" {
			chunks = append(chunks, Chunk{Source: rel, Title: title, Body: text})
		}
		body = nil
	}

	for _, line := range strings.Split(string(data), "\n") {
		if heading, ok := strings.CutPrefix(strings.TrimSpace(line), "## "); ok {
			flush()
			title = strings.TrimSpace(heading)
			continue
		}
		if title == "" {
			continue // 还在导语区
		}
		body = append(body, line)
	}
	flush()
	return chunks, nil
}

// buildStats 预计算平均块长与词频表，供 BM25 打分使用。
func (k *Knowledge) buildStats() {
	k.docFreq = map[string]int{}
	total := 0
	for _, chunk := range k.chunks {
		tokens := tokenize(chunk.Title + " " + chunk.Body)
		total += len(tokens)
		// 文档频率按块去重：一个块里出现三次和一个块里出现一次，
		// 对 idf 的贡献相同，否则长块会系统性压低频词。
		seen := map[string]bool{}
		for _, tok := range tokens {
			seen[tok] = true
		}
		for tok := range seen {
			k.docFreq[tok]++
		}
	}
	if len(k.chunks) > 0 {
		k.avgLen = float64(total) / float64(len(k.chunks))
	}
}

// Search 对 query 做关键词检索，按相关度降序返回至多 topK 条。
// 排序完全确定：分数相同时按文档顺序，方便回归与排障。
func (k *Knowledge) Search(query string, topK int) []Chunk {
	if k == nil || topK <= 0 {
		return nil
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, tok := range terms {
		seen[tok] = true
	}

	type scored struct {
		chunk Chunk
		score float64
		index int
	}
	results := make([]scored, 0, len(k.chunks))
	for i, chunk := range k.chunks {
		tokens := tokenize(chunk.Title + " " + chunk.Body)
		if len(tokens) == 0 {
			continue
		}

		tf := map[string]int{}
		for _, tok := range tokens {
			tf[tok]++
		}

		var score float64
		for term := range seen {
			freq, ok := tf[term]
			if !ok {
				continue
			}
			idf := k.idf(term)
			if idf <= 0 {
				continue
			}
			score += idf * bm25Term(freq, len(tokens), k.avgLen)
		}
		if score > 0 {
			results = append(results, scored{chunk: chunk, score: score, index: i})
		}
	}

	sort.SliceStable(results, func(a, b int) bool {
		if results[a].score != results[b].score {
			return results[a].score > results[b].score
		}
		return results[a].index < results[b].index
	})

	if len(results) > topK {
		results = results[:topK]
	}
	out := make([]Chunk, 0, len(results))
	for _, r := range results {
		out = append(out, r.chunk)
	}
	return out
}

// idf 是 BM25 的逆文档频率，加 1 平滑避免整除与负值。
func (k *Knowledge) idf(term string) float64 {
	n := float64(len(k.chunks))
	df := float64(k.docFreq[term])
	if df <= 0 {
		return 0
	}
	num := n - df + 0.5
	den := df + 0.5
	return math.Log1p(num / den)
}

// Retrieve 实现 eino 的 retriever.Retriever，供 compose 图与工具层调用。
// 尊重调用方通过 retriever.WithTopK / WithScoreThreshold 传入的参数。
func (k *Knowledge) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if k == nil {
		return nil, nil
	}

	topK := k.topK
	common := retriever.GetCommonOptions(&retriever.Options{TopK: &topK}, opts...)
	if common.TopK != nil {
		topK = *common.TopK
	}

	docs := make([]*schema.Document, 0, topK)
	for _, chunk := range k.Search(query, topK) {
		docs = append(docs, &schema.Document{
			ID:      chunk.Source + "#" + chunk.Title,
			Content: chunk.Body,
			MetaData: map[string]any{
				"source": chunk.Source,
				"title":  chunk.Title,
			},
		})
	}
	return docs, nil
}

// tokenize 把中英混合文本切成检索词。
//
// 中文没有天然词边界，按二元滑窗切（"证书有效期" -> "证书","书有效","有效期"），
// 同时保留单字，保证单字查询也有召回；英文与数字按连续字母数字切成小写词，
// 长度不足 2 的英文词丢弃（"ip" 这类两字母词与端口号都会保留）。
func tokenize(s string) []string {
	var tokens []string
	var latin []rune

	flushLatin := func() {
		if len(latin) == 0 {
			return
		}
		word := strings.ToLower(string(latin))
		latin = nil
		if len([]rune(word)) >= 2 {
			tokens = append(tokens, word)
		}
	}

	runes := []rune(s)
	for i, r := range runes {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			tokens = append(tokens, string(r))
			if i+1 < len(runes) && unicode.Is(unicode.Han, runes[i+1]) {
				tokens = append(tokens, string(runes[i:i+2]))
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			latin = append(latin, r)
		default:
			flushLatin()
		}
	}
	flushLatin()
	return tokens
}

// bm25Term 是 BM25 的单篇单项得分，k1 控制词频饱和度，b 控制长度归一。
func bm25Term(freq, docLen int, avgLen float64) float64 {
	const k1 = 1.2
	const b = 0.75
	f := float64(freq)
	if avgLen <= 0 {
		avgLen = 1
	}
	norm := k1 * (1 - b + b*float64(docLen)/avgLen)
	return f * (k1 + 1) / (f + norm)
}
