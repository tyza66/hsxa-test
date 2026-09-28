package fofa

import (
	"context"
	"testing"
)

// testBundle 装载仓库内真实的 Skill 目录。测试直接调 Python 桥，
// 这样钉住的是"Go 侧对 convert.py / lookup.py 输出契约的理解"，
// 而不是一份自己编造的假实现。
func testBundle(t *testing.T) *Bundle {
	t.Helper()
	bundle, err := Load(LoadOptions{Probe: true})
	if err != nil {
		t.Fatalf("装载 Skill 失败: %v", err)
	}
	return bundle
}

// TestSkillConvertSuccess 钉死正常直译：退出码 0，首行即查询语句。
func TestSkillConvertSuccess(t *testing.T) {
	bundle := testBundle(t)
	conv, err := bundle.Skill().Convert(context.Background(), "请查询 IP 地址为 20.247.40.92 的资产。")
	if err != nil {
		t.Fatalf("Convert 报错: %v", err)
	}
	if !conv.OK {
		t.Fatalf("期望机械层可转换，实际被拒绝: note=%q", conv.Note)
	}
	if conv.Query != `ip="20.247.40.92"` {
		t.Errorf("查询语句不符: %q", conv.Query)
	}
	if conv.Confidence != "H" {
		t.Errorf("置信度期望 H，实际 %q", conv.Confidence)
	}
	if conv.Note == "" {
		t.Error("机械层应给出说明")
	}
}

// TestSkillConvertDeclined 钉死"拒绝不是错误"：退出码 1 加"机械层不处理"前缀。
func TestSkillConvertDeclined(t *testing.T) {
	bundle := testBundle(t)
	conv, err := bundle.Skill().Convert(context.Background(), "帮我查最安全的网站。")
	if err != nil {
		t.Fatalf("Convert 报错（拒绝不应被当成错误）: %v", err)
	}
	if conv.OK {
		t.Fatalf("期望机械层拒绝，实际转换成功: query=%q", conv.Query)
	}
	if conv.Query != "" {
		t.Errorf("拒绝时查询语句应为空: %q", conv.Query)
	}
	if conv.Note == "" {
		t.Error("拒绝时应带说明，供链路与模型判据使用")
	}
}

// TestSkillConvertFixedAnswer 钉死无解题：固定答案算转换成功且置信度 H，
// 这是 convert.py 单句模式曾把固定答案送进 lint 而误报体检失败的暗病现场。
func TestSkillConvertFixedAnswer(t *testing.T) {
	bundle := testBundle(t)
	conv, err := bundle.Skill().Convert(context.Background(), "搜索开放 1000000 端口的资产。")
	if err != nil {
		t.Fatalf("Convert 报错: %v", err)
	}
	if !conv.OK {
		t.Fatalf("固定答案也应算转换成功，实际 OK=false note=%q", conv.Note)
	}
	if conv.Query != FixedAnswer {
		t.Errorf("固定答案不符: %q", conv.Query)
	}
	if conv.Confidence != "H" {
		t.Errorf("置信度期望 H，实际 %q", conv.Confidence)
	}
}

// TestSkillLint 钉死语法体检的通过与不通过两条路径。
func TestSkillLint(t *testing.T) {
	bundle := testBundle(t)

	issues, err := bundle.Skill().Lint(context.Background(), `ip="1.1.1.1"`)
	if err != nil {
		t.Fatalf("Lint 合法语句报错: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("合法语句不应有问题: %v", issues)
	}

	// port 的值必须加引号，这是 Skill 里反复强调的易错点。
	issues, err = bundle.Skill().Lint(context.Background(), `port=80`)
	if err != nil {
		t.Fatalf("Lint 非法语句报错: %v", err)
	}
	if len(issues) == 0 {
		t.Error("port=80 应判体检未通过")
	}
}
