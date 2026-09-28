package fofa

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/retriever"
)

// TestKnowledgeIndexBuilt 确认索引把 SKILL.md 与 reference/ 都吃进去了。
func TestKnowledgeIndexBuilt(t *testing.T) {
	kb := testBundle(t).Knowledge()
	if len(kb.chunks) < 10 {
		t.Fatalf("知识块数偏少，索引可能不完整: %d", len(kb.chunks))
	}

	sources := map[string]bool{}
	for _, chunk := range kb.chunks {
		sources[chunk.Source] = true
	}
	if !sources["SKILL.md"] {
		t.Error("索引里没有 SKILL.md 的知识块")
	}
	if !sources["reference/mapping.md"] {
		t.Error("索引里没有 reference/mapping.md 的知识块")
	}
}

// TestKnowledgeSearchRelevant 用"证书"这类有专门小节的主题检索，
// 命中的片段必须真的谈到证书，否则检索就是个摆设。
func TestKnowledgeSearchRelevant(t *testing.T) {
	kb := testBundle(t).Knowledge()

	hits := kb.Search("证书到期时间怎么写", 3)
	if len(hits) == 0 {
		t.Fatal("证书类查询无命中")
	}
	if !strings.Contains(hits[0].Title+hits[0].Body, "证书") {
		t.Errorf("首位命中与证书无关: %+v", hits[0])
	}
}

// TestKnowledgeSearchEdgeCases 空查询与非法 topK 都不该炸，直接返回空。
func TestKnowledgeSearchEdgeCases(t *testing.T) {
	kb := testBundle(t).Knowledge()

	if hits := kb.Search("   ", 3); len(hits) != 0 {
		t.Errorf("空查询应返回空: %+v", hits)
	}
	if hits := kb.Search("证书", 0); len(hits) != 0 {
		t.Errorf("topK<=0 应返回空: %+v", hits)
	}
	if hits := kb.Search("证书", -1); len(hits) != 0 {
		t.Errorf("topK<=0 应返回空: %+v", hits)
	}
}

// TestKnowledgeSearchDeterministic 同一查询两次结果必须一致：
// 排序稳定是这条链路可回归、可复现的前提。
func TestKnowledgeSearchDeterministic(t *testing.T) {
	kb := testBundle(t).Knowledge()

	first := kb.Search("请查询 IP 地址为 20.247.40.92 的资产", 5)
	second := kb.Search("请查询 IP 地址为 20.247.40.92 的资产", 5)
	if len(first) != len(second) {
		t.Fatalf("两次检索条数不一致: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Source != second[i].Source || first[i].Title != second[i].Title {
			t.Fatalf("第 %d 条不一致: %+v vs %+v", i, first[i], second[i])
		}
	}
}

// TestKnowledgeRetrieveTopK 验证 eino retriever 实现对调用方 TopK 的尊重，
// 以及 MetaData 与文档 ID 的契约（人工复核时要靠这些按图索骥）。
func TestKnowledgeRetrieveTopK(t *testing.T) {
	kb := testBundle(t).Knowledge()

	docs, err := kb.Retrieve(context.Background(), "证书", retriever.WithTopK(2))
	if err != nil {
		t.Fatalf("Retrieve 报错: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("期望 2 条文档，实际 %d", len(docs))
	}
	for _, doc := range docs {
		if doc.ID == "" || strings.Contains(doc.ID, "#") == false {
			t.Errorf("文档 ID 应为 来源#标题 形态: %q", doc.ID)
		}
		if doc.MetaData["source"] == nil || doc.MetaData["title"] == nil {
			t.Errorf("文档缺少 source/title 元数据: %+v", doc.MetaData)
		}
		if doc.Content == "" {
			t.Error("文档内容为空")
		}
	}
}

// TestTokenizeMixed 钉住中英混的分词约定：中文单字加二元滑窗，
// 英文数字按连续串小写，碎片（长度 1 的英文）丢弃。
func TestTokenizeMixed(t *testing.T) {
	tokens := tokenize(`查询 ip="1.1.1.1" 端口 80`)
	joined := "|" + strings.Join(tokens, "|") + "|"

	for _, want := range []string{
		"|ip|", // 英文连续串
		"|端|",  // 中文单字
		"|端口|", // 中文二元滑窗
		"|80|", // 纯数字词
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("分词结果缺少 %s: %s", want, joined)
		}
	}
	// 1.1.1.1 里的点号是切分点，逐段得到的 "1" 长度不足 2 被丢弃，
	// 这是刻意行为：知识库里 IP 本身不作为检索词出现。
	if strings.Contains(joined, "|1.1|") {
		t.Errorf("标点不应成为词的一部分: %s", joined)
	}
	if strings.Contains(joined, "|=|") {
		t.Errorf("引号不应成词: %s", joined)
	}
}
