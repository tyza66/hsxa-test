package fofa

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

// toolBodyLimit 是知识工具返回的单块正文字数上限（按 rune 计），
// 与链路注入 Prompt 时的截断长度保持一致。
const toolBodyLimit = 600

// convertInput 是 fofa_convert 工具的入参。
type convertInput struct {
	Requirement string `json:"requirement" jsonschema:"description=The natural-language asset search requirement to convert into a FOFA query, written in the user's language. Required."`
}

// convertReport 是 fofa_convert 工具的结构化返回值。
type convertReport struct {
	Convertible bool   `json:"convertible"` // 机械层是否接受了这个需求
	Query       string `json:"query"`       // 查询语句，不可转换时为空
	Confidence  string `json:"confidence"`  // 机械层置信度：H / M / L
	Note        string `json:"note"`        // 机械层的判定说明
}

// knowledgeInput 是 fofa_knowledge 工具的入参。
type knowledgeInput struct {
	Keyword string `json:"keyword" jsonschema:"description=Keyword or short phrase to look up in the FOFA Skill knowledge base, for example 'certificate expired' or Chinese keywords. Required."`
	TopK    int    `json:"top_k,omitempty" jsonschema:"description=Maximum number of knowledge snippets to return. Defaults to the configured knowledge top-k."`
}

// knowledgeHit 是知识库检索命中一条的结构化视图。
type knowledgeHit struct {
	Source string `json:"source"` // 来源文档，如 SKILL.md、reference/syntax.md
	Title  string `json:"title"`  // 所属二级标题
	Body   string `json:"body"`   // 正文（已截断）
}

// knowledgeReport 是 fofa_knowledge 工具的结构化返回值。
type knowledgeReport struct {
	Keyword string         `json:"keyword"`
	Hits    []knowledgeHit `json:"hits"`
}

// NewTools 返回 FOFA 链路暴露给模型的工具集合：
// fofa_convert（自然语言转查询语句）与 fofa_knowledge（查判据知识库）。
func NewTools(bundle *Bundle) ([]tool.BaseTool, error) {
	if bundle == nil || bundle.skill == nil || bundle.knowledge == nil {
		return nil, fmt.Errorf("fofa: Bundle 未正确装载，无法构建工具")
	}

	convertTool, err := newConvertTool(bundle.skill)
	if err != nil {
		return nil, err
	}
	knowledgeTool, err := newKnowledgeTool(bundle.knowledge)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{convertTool, knowledgeTool}, nil
}

// newConvertTool 把 Skill 机械层包装成模型可调用的工具。
func newConvertTool(s *Skill) (tool.InvokableTool, error) {
	t, err := toolutils.InferTool(
		"fofa_convert",
		"Convert a natural-language asset search requirement into a FOFA query statement. "+
			"Use this whenever the user asks to find assets on FOFA. "+
			"The conversion is deterministic and already validated; when it reports "+
			"convertible=false, answer with exactly this fixed string: "+FixedAnswer+".",
		func(ctx context.Context, in *convertInput) (*convertReport, error) {
			if in == nil || strings.TrimSpace(in.Requirement) == "" {
				return nil, fmt.Errorf("fofa_convert: requirement 不能为空")
			}

			conv, err := s.Convert(ctx, in.Requirement)
			if err != nil {
				return nil, err
			}
			if !conv.OK {
				return &convertReport{Convertible: false, Note: conv.Note}, nil
			}
			return &convertReport{
				Convertible: true,
				Query:       conv.Query,
				Confidence:  conv.Confidence,
				Note:        conv.Note,
			}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("fofa: 构建 fofa_convert 工具失败: %w", err)
	}
	return t, nil
}

// newKnowledgeTool 把 Skill 知识库检索包装成模型可调用的工具。
func newKnowledgeTool(k *Knowledge) (tool.InvokableTool, error) {
	t, err := toolutils.InferTool(
		"fofa_knowledge",
		"Search the FOFA Skill knowledge base for syntax rules, the field table and "+
			"conversion conventions. Consult this before composing a query yourself, "+
			"to check field names, operators and known edge cases.",
		func(_ context.Context, in *knowledgeInput) (*knowledgeReport, error) {
			if in == nil || strings.TrimSpace(in.Keyword) == "" {
				return nil, fmt.Errorf("fofa_knowledge: keyword 不能为空")
			}

			topK := in.TopK
			if topK <= 0 {
				topK = k.topK
			}
			hits := make([]knowledgeHit, 0, topK)
			for _, chunk := range k.Search(in.Keyword, topK) {
				hits = append(hits, knowledgeHit{
					Source: chunk.Source,
					Title:  chunk.Title,
					Body:   truncate(chunk.Body, toolBodyLimit),
				})
			}
			return &knowledgeReport{
				Keyword: strings.TrimSpace(in.Keyword),
				Hits:    hits,
			}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("fofa: 构建 fofa_knowledge 工具失败: %w", err)
	}
	return t, nil
}
