package fofa

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// graphName 是 eino 图上这条链路的名标识，跑在 trace 日志里。
const graphName = "hsxa-fofa-chain"

// 分支节点名。
const (
	nodeMechanical = "mechanical" // 机械层直接转换
	nodeModel      = "model"      // 模型基于知识判据生成
	nodeDeclined   = "declined"   // 放弃并给固定答案
)

// defaultPersona 是模型判据步骤的默认系统人设，可被 FOFA_PERSONA 覆盖。
const defaultPersona = "你是 FOFA 检索语句构造专家，只依据给定参考资料把自然语言资产搜索需求" +
	"转换成一条合法的 FOFA 查询语句。\n" +
	"输出契约：\n" +
	"1. 只输出一行查询语句本身，不要解释、不要 Markdown 代码块、不要多个候选；\n" +
	"2. 需求确实无法转换时，原样输出固定串：" + FixedAnswer + "；\n" +
	"3. 字段与运算符以参考资料为准，参考资料没提的字段不要用。"

// Ref 指向答案依据的知识片段，供人工复核时按图索骥。
type Ref struct {
	Source string `json:"来源"`
	Title  string `json:"标题"`
}

// Answer 是一条 FOFA 结构化答案，字段名与 Skill 答卷保持一致。
type Answer struct {
	Question   string   `json:"问题"`
	Query      string   `json:"查询语句"`
	Confidence string   `json:"置信度"` // H 高 / M 中 / L 低
	Note       string   `json:"说明"`
	Source     string   `json:"来源"` // mechanical / model / declined
	LintIssues []string `json:"体检问题,omitempty"`
	Candidate  string   `json:"候选语句,omitempty"` // 模型原始候选，被体检拦下时留档
	Knowledge  []Ref    `json:"知识依据,omitempty"`
}

// fofaContext 是链路内部流转的上下文：需求、命中的知识、机械层结论与待发消息。
type fofaContext struct {
	Question   string
	Refs       []Ref
	Chunks     []Chunk
	Mechanical Conversion
	HasModel   bool
	Messages   []*schema.Message
}

// Chain 是"自然语言需求 -> 结构化 FOFA 答案"的链式编排。
//
// 固定链路，不做多轮规划：
//  1. prepare：检索 Skill 知识库，并先让机械层试一次（确定性）；
//  2. 分支：机械层成了就直接收口；没成且有模型，才把知识与机械层说明
//     一起交给模型判据；两者都没有，按固定答案收口；
//  3. 收口：模型候选一律过 Skill 语法体检，不过就改判固定答案并留档。
type Chain struct {
	bundle  *Bundle
	model   model.ToolCallingChatModel
	topK    int
	persona string
	logger  *slog.Logger

	runnable compose.Runnable[string, *Answer]
}

// NewChain 装配 FOFA 链路。chatModel 为 nil 时模型判据步骤不可用，
// 链路退化为"机械层 + 固定答案"，其余行为不变。
//
// persona 非空时覆盖模型判据步骤的内置系统人设（对应 FOFA_PERSONA）。
func NewChain(bundle *Bundle, chatModel model.ToolCallingChatModel, persona string, logger *slog.Logger) (*Chain, error) {
	if bundle == nil || bundle.knowledge == nil || bundle.skill == nil {
		return nil, errors.New("fofa: Bundle 未正确装载，无法构建链路")
	}
	if logger == nil {
		logger = slog.Default()
	}

	c := &Chain{
		bundle:  bundle,
		model:   chatModel,
		topK:    bundle.knowledge.topK,
		persona: persona,
		logger:  logger,
	}

	chain := compose.NewChain[string, *Answer]()
	chain.AppendLambda(compose.InvokableLambda(c.prepare))

	// 分支节点要先逐个挂到 ChainBranch 上，再把整个分支追加进 chain。
	branch := compose.NewChainBranch[*fofaContext](c.route)
	branch.AddLambda(nodeMechanical, compose.InvokableLambda(c.byMechanical))
	branch.AddLambda(nodeModel, compose.InvokableLambda(c.byModel))
	branch.AddLambda(nodeDeclined, compose.InvokableLambda(c.byDeclined))
	chain.AppendBranch(branch)

	runnable, err := chain.Compile(context.Background(), compose.WithGraphName(graphName))
	if err != nil {
		return nil, fmt.Errorf("fofa: 编译链路失败: %w", err)
	}
	c.runnable = runnable
	return c, nil
}

// Run 把一句自然语言需求跑完整条链路。
func (c *Chain) Run(ctx context.Context, question string) (*Answer, error) {
	if c == nil || c.runnable == nil {
		return nil, errors.New("fofa: 链路未编译")
	}
	return c.runnable.Invoke(ctx, question)
}

// prepare 是链路第一个节点：检索知识、试机械层、备好模型消息。
func (c *Chain) prepare(ctx context.Context, question string) (*fofaContext, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, errors.New("fofa: 需求为空")
	}

	chunks := c.bundle.knowledge.Search(question, c.topK)
	refs := make([]Ref, 0, len(chunks))
	for _, chunk := range chunks {
		refs = append(refs, Ref{Source: chunk.Source, Title: chunk.Title})
	}

	conv, err := c.bundle.skill.Convert(ctx, question)
	if err != nil {
		c.logger.ErrorContext(ctx, "fofa: 机械层调用失败", "question", question, "error", err)
		return nil, err
	}

	c.logger.InfoContext(ctx, "fofa: 链路准备完成",
		"question", question,
		"知识命中", len(refs),
		"机械层可转换", conv.OK,
		"模型可用", c.model != nil,
	)

	return &fofaContext{
		Question:   question,
		Refs:       refs,
		Chunks:     chunks,
		Mechanical: conv,
		HasModel:   c.model != nil,
		Messages:   c.buildMessages(question, conv, chunks),
	}, nil
}

// route 决定走哪个分支：机械层优先，模型兜底，都没有才放弃。
func (c *Chain) route(_ context.Context, in *fofaContext) (string, error) {
	if in.Mechanical.OK {
		return nodeMechanical, nil
	}
	if in.HasModel {
		return nodeModel, nil
	}
	return nodeDeclined, nil
}

// byMechanical 机械层分支：原样带出查询语句、置信度与说明。
func (c *Chain) byMechanical(_ context.Context, in *fofaContext) (*Answer, error) {
	return &Answer{
		Question:   in.Question,
		Query:      in.Mechanical.Query,
		Confidence: in.Mechanical.Confidence,
		Note:       in.Mechanical.Note,
		Source:     SourceMechanical,
		Knowledge:  in.Refs,
	}, nil
}

// byModel 模型分支：让模型基于知识判据给候选，再过体检收口。
func (c *Chain) byModel(ctx context.Context, in *fofaContext) (*Answer, error) {
	resp, err := c.model.Generate(ctx, in.Messages)
	if err != nil {
		c.logger.ErrorContext(ctx, "fofa: 模型调用失败", "question", in.Question, "error", err)
		return nil, fmt.Errorf("fofa: 模型调用失败: %w", err)
	}
	if resp == nil {
		return nil, errors.New("fofa: 模型返回空消息")
	}

	candidate := extractQuery(resp.Content)
	answer := &Answer{
		Question:  in.Question,
		Source:    SourceModel,
		Knowledge: in.Refs,
	}

	if candidate == "" {
		// 模型没给出可解析的语句，不敢替它补，按固定答案收口。
		answer.Query = FixedAnswer
		answer.Confidence = "L"
		answer.Note = "模型未给出可解析的查询语句，按 Skill 红线改判固定答案"
		c.logger.WarnContext(ctx, "fofa: 模型输出无法解析", "question", in.Question, "raw", resp.Content)
		return answer, nil
	}
	if candidate == FixedAnswer {
		answer.Query = FixedAnswer
		answer.Confidence = "H"
		answer.Note = joinNotes("模型判定不可转换", in.Mechanical.Note)
		return answer, nil
	}

	issues, err := c.bundle.skill.Lint(ctx, candidate)
	if err != nil {
		return nil, err
	}
	if len(issues) > 0 {
		// 宁可不答：体检不过的语句直接改判固定答案，候选留档备查。
		answer.Query = FixedAnswer
		answer.Confidence = "L"
		answer.LintIssues = issues
		answer.Candidate = candidate
		answer.Note = fmt.Sprintf("模型候选未通过语法体检（%s），按 Skill 红线改判固定答案",
			strings.Join(issues, "；"))
		c.logger.WarnContext(ctx, "fofa: 模型候选未过体检", "question", in.Question, "candidate", candidate)
		return answer, nil
	}

	answer.Query = candidate
	answer.Confidence = "M"
	answer.Note = joinNotes("模型基于知识判据生成，已通过语法体检", in.Mechanical.Note)
	return answer, nil
}

// byDeclined 放弃分支：机械层不处理且没有模型可用，给固定答案并说清原因。
func (c *Chain) byDeclined(_ context.Context, in *fofaContext) (*Answer, error) {
	return &Answer{
		Question:   in.Question,
		Query:      FixedAnswer,
		Confidence: "L",
		Note: joinNotes("机械层不处理，且未配置模型凭据，模型判据步骤不可用",
			in.Mechanical.Note),
		Source:    SourceDeclined,
		Knowledge: in.Refs,
	}, nil
}

// knowledgeBodyLimit 是单条知识片段注入 Prompt 的最大字数（按 rune 计）。
// 判据文档一块通常几十行，截断是为了把 Prompt 体积稳住，细节让模型按需追问。
const knowledgeBodyLimit = 600

// buildMessages 组装模型判据步骤的Prompt：人设 + 需求 + 知识 + 机械层说明 + 输出契约。
func (c *Chain) buildMessages(question string, conv Conversion, chunks []Chunk) []*schema.Message {
	var knowledge strings.Builder
	if len(chunks) > 0 {
		for i, chunk := range chunks {
			fmt.Fprintf(&knowledge, "%d. [%s / %s]\n%s\n", i+1, chunk.Source, chunk.Title,
				truncate(chunk.Body, knowledgeBodyLimit))
		}
	} else {
		knowledge.WriteString("（未命中相关知识）")
	}

	mechanical := conv.Note
	if mechanical == "" {
		mechanical = "机械层拒绝了本次转换"
	}

	user := fmt.Sprintf("需求：%s\n\n机械层说明：%s\n\n相关知识片段：\n%s\n\n请输出最终查询语句。",
		question, mechanical, knowledge.String())

	persona := defaultPersona
	if c.persona != "" {
		persona = c.persona
	}

	return []*schema.Message{
		{
			Role:    schema.System,
			Content: persona,
		},
		{Role: schema.User, Content: user},
	}
}

// extractQuery 从模型回复里抠出查询语句：
// 去代码块围栏、去常见前缀标签、取第一个非空行。
func extractQuery(content string) string {
	text := strings.TrimSpace(content)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		var inner []string
		for _, line := range lines[1:] {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				break
			}
			inner = append(inner, line)
		}
		text = strings.TrimSpace(strings.Join(inner, "\n"))
	}

	line := ""
	for _, candidate := range strings.Split(text, "\n") {
		if strings.TrimSpace(candidate) != "" {
			line = strings.TrimSpace(candidate)
			break
		}
	}
	if line == "" {
		return ""
	}

	// 模型爱加"查询语句：xxx"这类标签，剥掉再比对外层语义。
	for _, prefix := range []string{
		"查询语句：", "查询语句:", "查询语句 ", "FOFA查询语句：", "FOFA查询语句:",
		"FOFA：", "FOFA:", "结果：", "结果:", "答案：", "答案:",
	} {
		if trimmed, ok := strings.CutPrefix(line, prefix); ok {
			line = strings.TrimSpace(trimmed)
			break
		}
	}
	return trimWrappers(line)
}

// trimWrappers 去掉模型包在语句外面的壳：反引号、空白可直删，
// 引号必须成对剥一层。FOFA 语句自身常以引号结尾（如 ip="1.1.1.1"），
// 无脑 Trim 首尾引号会把语句咬坏，再被体检误杀。
func trimWrappers(line string) string {
	line = strings.Trim(line, "` ")
	if len(line) >= 2 {
		if quote := line[0]; (quote == '"' || quote == '\'') && line[len(line)-1] == quote {
			line = strings.Trim(line[1:len(line)-1], "` ")
		}
	}
	return line
}

// joinNotes 用分号拼接说明，跳过空段。
func joinNotes(parts ...string) string {
	var kept []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, strings.TrimSpace(part))
		}
	}
	return strings.Join(kept, "；")
}

// truncate 把长文本压到 limit 个 rune 以内：只折叠连续空行，保留 markdown
// 表格与列表的行结构，超长时以省略号收尾并保证是合法 UTF-8。
func truncate(s string, limit int) string {
	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	prevBlank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if prevBlank {
				continue
			}
			prevBlank = true
		} else {
			prevBlank = false
		}
		kept = append(kept, line)
	}
	text := strings.TrimSpace(strings.Join(kept, "\n"))
	if len([]rune(text)) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "..."
}
