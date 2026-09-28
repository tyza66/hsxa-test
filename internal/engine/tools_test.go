package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/tool"

	"github.com/tyza66/hsxa-test/internal/fofa"
)

// loadTestBundle 装载仓库内真实 Skill，供引擎层的工具测试复用。
func loadTestBundle(t *testing.T) *fofa.Bundle {
	t.Helper()
	bundle, err := fofa.Load(fofa.LoadOptions{Probe: true})
	if err != nil {
		t.Fatalf("装载 Skill 失败: %v", err)
	}
	return bundle
}

// toolNames 取工具集合的名字列表，Info 失败直接让测试失败。
func toolNames(t *testing.T, items []tool.BaseTool) []string {
	t.Helper()
	names := make([]string, 0, len(items))
	for _, item := range items {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatalf("读取工具信息失败: %v", err)
		}
		names = append(names, info.Name)
	}
	return names
}

func hasName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// TestNewToolsWithBundle 装载 FOFA Skill 后，工具集应在系统工具之外
// 追加转换与知识检索两个工具。
func TestNewToolsWithBundle(t *testing.T) {
	items, err := newTools(loadTestBundle(t))
	if err != nil {
		t.Fatalf("构造工具集失败: %v", err)
	}
	names := toolNames(t, items)
	if len(names) != 3 {
		t.Fatalf("期望 3 个工具，实际 %d: %v", len(names), names)
	}
	for _, want := range []string{"current_time", "fofa_convert", "fofa_knowledge"} {
		if !hasName(names, want) {
			t.Errorf("缺少工具 %s: %v", want, names)
		}
	}
}

// TestNewToolsWithoutBundle 未装载 FOFA 链路时只保留系统工具，
// 路由层面也随之不注册 /v1/fofa。
func TestNewToolsWithoutBundle(t *testing.T) {
	items, err := newTools(nil)
	if err != nil {
		t.Fatalf("构造工具集失败: %v", err)
	}
	names := toolNames(t, items)
	if len(names) != 1 || names[0] != "current_time" {
		t.Fatalf("未装载时应只有 current_time，实际 %v", names)
	}
}

// TestFofaConvertToolRun 真调一次 fofa_convert：入参 JSON 进、结构化结果出。
func TestFofaConvertToolRun(t *testing.T) {
	items, err := newTools(loadTestBundle(t))
	if err != nil {
		t.Fatalf("构造工具集失败: %v", err)
	}
	convertTool := findInvokable(t, items, "fofa_convert")

	raw, err := convertTool.InvokableRun(context.Background(),
		`{"requirement":"请查询 IP 地址为 20.247.40.92 的资产。"}`)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}
	var report struct {
		Convertible bool   `json:"convertible"`
		Query       string `json:"query"`
		Confidence  string `json:"confidence"`
		Note        string `json:"note"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("工具返回不是合法 JSON: %v, raw=%s", err, raw)
	}
	if !report.Convertible || report.Query != `ip="20.247.40.92"` {
		t.Errorf("转换结果不符: %+v", report)
	}
}

// TestFofaConvertToolDeclined 机械层拒绝时工具如实上报 convertible=false。
func TestFofaConvertToolDeclined(t *testing.T) {
	items, err := newTools(loadTestBundle(t))
	if err != nil {
		t.Fatalf("构造工具集失败: %v", err)
	}
	convertTool := findInvokable(t, items, "fofa_convert")

	raw, err := convertTool.InvokableRun(context.Background(),
		`{"requirement":"帮我查最安全的网站。"}`)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}
	var report struct {
		Convertible bool   `json:"convertible"`
		Query       string `json:"query"`
		Note        string `json:"note"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("工具返回不是合法 JSON: %v, raw=%s", err, raw)
	}
	if report.Convertible {
		t.Errorf("机械层拒绝时 convertible 应为 false: %+v", report)
	}
	if report.Query != "" {
		t.Errorf("拒绝时 query 应为空: %+v", report)
	}
}

// TestFofaKnowledgeToolRun 真调一次 fofa_knowledge：关键词命中应带来源与正文。
func TestFofaKnowledgeToolRun(t *testing.T) {
	items, err := newTools(loadTestBundle(t))
	if err != nil {
		t.Fatalf("构造工具集失败: %v", err)
	}
	knowledgeTool := findInvokable(t, items, "fofa_knowledge")

	raw, err := knowledgeTool.InvokableRun(context.Background(), `{"keyword":"证书"}`)
	if err != nil {
		t.Fatalf("调用工具失败: %v", err)
	}
	var report struct {
		Keyword string `json:"keyword"`
		Hits    []struct {
			Source string `json:"source"`
			Title  string `json:"title"`
			Body   string `json:"body"`
		} `json:"hits"`
	}
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatalf("工具返回不是合法 JSON: %v, raw=%s", err, raw)
	}
	if len(report.Hits) == 0 {
		t.Fatal("证书关键词应有命中")
	}
	if report.Hits[0].Source == "" || report.Hits[0].Body == "" {
		t.Errorf("命中的片段应带来源与正文: %+v", report.Hits[0])
	}
}

// TestFofaKnowledgeToolBadInput 空关键词应被工具层拒绝，而不是查个空。
func TestFofaKnowledgeToolBadInput(t *testing.T) {
	items, err := newTools(loadTestBundle(t))
	if err != nil {
		t.Fatalf("构造工具集失败: %v", err)
	}
	knowledgeTool := findInvokable(t, items, "fofa_knowledge")
	if _, err := knowledgeTool.InvokableRun(context.Background(), `{"keyword":"  "}`); err == nil {
		t.Fatal("空关键词应报错")
	}
}

// findInvokable 按名字取工具并断言它是可调用的 InvokableTool。
func findInvokable(t *testing.T, items []tool.BaseTool, name string) tool.InvokableTool {
	t.Helper()
	for _, item := range items {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatalf("读取工具信息失败: %v", err)
		}
		if info.Name != name {
			continue
		}
		invokable, ok := item.(tool.InvokableTool)
		if !ok {
			t.Fatalf("工具 %s 不是 InvokableTool: %T", name, item)
		}
		return invokable
	}
	t.Fatalf("找不到工具 %s", name)
	return nil
}
