package fofa

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// fakeModel 是一个只记录调用、返回固定回复的假模型，
// 用来在没有真实模型凭据的环境里钉住链路的模型判据分支。
type fakeModel struct {
	reply     string // 每次 Generate 返回的正文
	err       error  // 非空时 Generate 直接返回该错误
	calls     int    // Generate 被调用次数
	lastInput []*schema.Message
}

func (f *fakeModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	f.lastInput = input
	if f.err != nil {
		return nil, f.err
	}
	return &schema.Message{Role: schema.Assistant, Content: f.reply}, nil
}

func (f *fakeModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("fakeModel 不支持流式")
}

func (f *fakeModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

// newTestChain 用真实 Skill 装配一条链路，模型可传 nil。
func newTestChain(t *testing.T, chatModel model.ToolCallingChatModel) *Chain {
	t.Helper()
	chain, err := NewChain(testBundle(t), chatModel, "", nil)
	if err != nil {
		t.Fatalf("构建链路失败: %v", err)
	}
	return chain
}

// TestChainMechanical 机械层直译路径：不该惊动模型，查询语句原样带出。
func TestChainMechanical(t *testing.T) {
	fake := &fakeModel{reply: `ip="0.0.0.0"`}
	chain := newTestChain(t, fake)

	answer, err := chain.Run(context.Background(), "请查询 IP 地址为 20.247.40.92 的资产。")
	if err != nil {
		t.Fatalf("链路执行失败: %v", err)
	}
	if answer.Source != SourceMechanical {
		t.Errorf("来源期望 %s，实际 %s", SourceMechanical, answer.Source)
	}
	if answer.Query != `ip="20.247.40.92"` {
		t.Errorf("查询语句不符: %q", answer.Query)
	}
	if answer.Confidence != "H" {
		t.Errorf("置信度期望 H，实际 %q", answer.Confidence)
	}
	if fake.calls != 0 {
		t.Errorf("机械层成了就不该调模型，实际调了 %d 次", fake.calls)
	}
	if len(answer.Knowledge) == 0 {
		t.Error("答案应带知识依据，供人工复核")
	}
}

// TestChainModel 模型判据路径：机械层拒绝时有模型兜底，
// 模型给出的合法候选应原样收口并标注来源。
func TestChainModel(t *testing.T) {
	fake := &fakeModel{reply: "查询语句：port=\"2087\""}
	chain := newTestChain(t, fake)

	answer, err := chain.Run(context.Background(), "帮我查最安全的网站。")
	if err != nil {
		t.Fatalf("链路执行失败: %v", err)
	}
	if answer.Source != SourceModel {
		t.Errorf("来源期望 %s，实际 %s", SourceModel, answer.Source)
	}
	if answer.Query != `port="2087"` {
		t.Errorf("查询语句不符: %q", answer.Query)
	}
	if len(answer.LintIssues) != 0 {
		t.Errorf("合法候选不应有体检问题: %v", answer.LintIssues)
	}
	if fake.calls != 1 {
		t.Errorf("模型应被调用一次，实际 %d 次", fake.calls)
	}
	if answer.Confidence != "M" {
		t.Errorf("模型路径置信度期望 M，实际 %q", answer.Confidence)
	}
}

// TestChainModelPrompt 钉住喂给模型的 Prompt 契约：人设、需求、
// 机械层说明、知识片段都要在，否则模型判据就是盲猜。
func TestChainModelPrompt(t *testing.T) {
	fake := &fakeModel{reply: `ip="1.1.1.1"`}
	chain := newTestChain(t, fake)

	_, err := chain.Run(context.Background(), "帮我查最安全的网站。")
	if err != nil {
		t.Fatalf("链路执行失败: %v", err)
	}
	if len(fake.lastInput) != 2 {
		t.Fatalf("期望 system+user 两条消息，实际 %d 条", len(fake.lastInput))
	}
	if fake.lastInput[0].Role != schema.System || !strings.Contains(fake.lastInput[0].Content, "FOFA") {
		t.Errorf("首条应为含 FOFA 的系统人设: %+v", fake.lastInput[0])
	}
	if fake.lastInput[1].Role != schema.User || !strings.Contains(fake.lastInput[1].Content, "帮我查最安全的网站。") {
		t.Errorf("用户消息应包含原始需求: %+v", fake.lastInput[1])
	}
}

// TestChainModelLintFailed 模型候选过不了体检时，宁可不答：
// 改判固定答案、留档候选、列出问题，而不是把坏语句当成功交出去。
func TestChainModelLintFailed(t *testing.T) {
	fake := &fakeModel{reply: "port=2087"} // 少了一对引号，体检必挂
	chain := newTestChain(t, fake)

	answer, err := chain.Run(context.Background(), "帮我查最安全的网站。")
	if err != nil {
		t.Fatalf("链路执行失败（体检不过应被收口，不是报错）: %v", err)
	}
	if answer.Source != SourceModel {
		t.Errorf("来源期望 %s，实际 %s", SourceModel, answer.Source)
	}
	if answer.Query != FixedAnswer {
		t.Errorf("体检不过应改判固定答案，实际 %q", answer.Query)
	}
	if len(answer.LintIssues) == 0 {
		t.Error("应列出体检问题")
	}
	if answer.Candidate != "port=2087" {
		t.Errorf("候选语句应留档: %q", answer.Candidate)
	}
	if answer.Confidence != "L" {
		t.Errorf("置信度期望 L，实际 %q", answer.Confidence)
	}
}

// TestChainModelEmpty 模型没给出可解析内容时同样按固定答案收口。
func TestChainModelEmpty(t *testing.T) {
	fake := &fakeModel{reply: "   \n\n  "}
	chain := newTestChain(t, fake)

	answer, err := chain.Run(context.Background(), "帮我查最安全的网站。")
	if err != nil {
		t.Fatalf("链路执行失败: %v", err)
	}
	if answer.Query != FixedAnswer {
		t.Errorf("空回复应改判固定答案，实际 %q", answer.Query)
	}
	if answer.Confidence != "L" {
		t.Errorf("置信度期望 L，实际 %q", answer.Confidence)
	}
}

// TestChainDeclined 机械层不处理且没有模型：给固定答案并说清原因。
func TestChainDeclined(t *testing.T) {
	chain := newTestChain(t, nil)

	answer, err := chain.Run(context.Background(), "帮我查最安全的网站。")
	if err != nil {
		t.Fatalf("链路执行失败: %v", err)
	}
	if answer.Source != SourceDeclined {
		t.Errorf("来源期望 %s，实际 %s", SourceDeclined, answer.Source)
	}
	if answer.Query != FixedAnswer {
		t.Errorf("查询语句应为固定答案，实际 %q", answer.Query)
	}
	if answer.Confidence != "L" {
		t.Errorf("置信度期望 L，实际 %q", answer.Confidence)
	}
	if !strings.Contains(answer.Note, "机械层不处理") {
		t.Errorf("说明应交代拒绝原因: %q", answer.Note)
	}
}

// TestChainModelError 模型调用失败必须如实上抛，不能伪装成固定答案。
func TestChainModelError(t *testing.T) {
	fake := &fakeModel{err: errors.New("上游 503")}
	chain := newTestChain(t, fake)

	if _, err := chain.Run(context.Background(), "帮我查最安全的网站。"); err == nil {
		t.Fatal("模型报错时链路应返回错误")
	}
}

// TestChainEmptyQuestion 空需求应在第一个节点就被拦下。
func TestChainEmptyQuestion(t *testing.T) {
	chain := newTestChain(t, nil)
	if _, err := chain.Run(context.Background(), "   "); err == nil {
		t.Fatal("空需求应报错")
	}
}

// TestNewChainNilBundle 装载不全的 Bundle 必须被拒绝，而不是留下
// 一个跑起来才空指针的链路。
func TestNewChainNilBundle(t *testing.T) {
	if _, err := NewChain(nil, nil, "", nil); err == nil {
		t.Fatal("nil Bundle 应拒绝构建")
	}
}

// TestExtractQuery 钉住从模型回复里抠语句的几条常见形态。
func TestExtractQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"裸语句", `ip="1.1.1.1"`, `ip="1.1.1.1"`},
		{"代码块围栏", "```\nip=\"1.1.1.1\"\n```", `ip="1.1.1.1"`},
		{"带语言标注的围栏", "```sql\nip=\"1.1.1.1\"\n```", `ip="1.1.1.1"`},
		{"前缀标签", "查询语句：ip=\"1.1.1.1\"", `ip="1.1.1.1"`},
		{"首行空行", "\n\n  ip=\"1.1.1.1\"  \n其他话", `ip="1.1.1.1"`},
		{"空输入", "  \n ", ""},
		{"纯围栏", "```\n```", ""},
		{"整层双引号包裹", `"ip="1.1.1.1""`, `ip="1.1.1.1"`},
		{"单引号包裹", `'ip="1.1.1.1"'`, `ip="1.1.1.1"`},
		{"反引号包裹", "`ip=\"1.1.1.1\"`", `ip="1.1.1.1"`},
		{"引号不成对不应剥", `"ip="1.1.1.1`, `"ip="1.1.1.1`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractQuery(c.in); got != c.want {
				t.Errorf("extractQuery(%q) = %q，期望 %q", c.in, got, c.want)
			}
		})
	}
}

// TestChainHelperFuncs 钉住两个收尾小工具的纯函数行为。
func TestChainHelperFuncs(t *testing.T) {
	if got := joinNotes("说明一", "", "  ", "说明二"); got != "说明一；说明二" {
		t.Errorf("joinNotes 不应保留空段: %q", got)
	}
	if got := joinNotes(); got != "" {
		t.Errorf("joinNotes 无输入应为空: %q", got)
	}
	if got := truncate("一二三四五", 3); got != "一二三..." {
		t.Errorf("truncate 应按 rune 截断并加省略号: %q", got)
	}
	if got := truncate("短文本", 100); got != "短文本" {
		t.Errorf("未超长不应改动: %q", got)
	}
}
