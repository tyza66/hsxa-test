package config

import (
	"testing"
	"time"
)

// clearEnv 清掉可能被外部环境污染的配置项，保证每个用例起点一致。
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SERVER_HOST", "SERVER_PORT", "SERVER_READ_TIMEOUT", "SERVER_WRITE_TIMEOUT", "SERVER_SHUTDOWN_TIMEOUT",
		"MODEL_PROVIDER", "MODEL_NAME", "MODEL_API_KEY", "MODEL_ACCESS_KEY", "MODEL_SECRET_KEY",
		"MODEL_BASE_URL", "MODEL_REGION", "MODEL_TEMPERATURE", "MODEL_TOP_P", "MODEL_MAX_TOKENS",
		"MODEL_RETRY_TIMES", "MODEL_TIMEOUT",
		"AGENT_ENABLED", "AGENT_MAX_STEP", "AGENT_PERSONA",
	} {
		t.Setenv(k, "")
	}
}

// minimalEnv 是能通过校验的最小环境变量组合。
func minimalEnv(t *testing.T) {
	t.Helper()
	clearEnv(t)
	t.Setenv("MODEL_NAME", "ep-test")
	t.Setenv("MODEL_API_KEY", "sk-test")
}

func TestLoadDefaults(t *testing.T) {
	minimalEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" || cfg.Server.Port != 8080 {
		t.Errorf("服务器默认值不对: host=%q port=%d", cfg.Server.Host, cfg.Server.Port)
	}
	if cfg.Server.Addr() != "0.0.0.0:8080" {
		t.Errorf("Addr()=%q", cfg.Server.Addr())
	}
	if cfg.Model.Provider != ProviderArk {
		t.Errorf("MODEL_PROVIDER 默认值=%q", cfg.Model.Provider)
	}
	if cfg.Agent.Enabled {
		t.Error("AGENT_ENABLED 默认应为 false")
	}
	if cfg.Agent.MaxStep != 12 {
		t.Errorf("AGENT_MAX_STEP 默认值=%d", cfg.Agent.MaxStep)
	}
	if cfg.Agent.Persona != DefaultPersona {
		t.Errorf("AGENT_PERSONA 默认值=%q", cfg.Agent.Persona)
	}
	if cfg.Server.WriteTimeout != 10*time.Minute {
		t.Errorf("SERVER_WRITE_TIMEOUT 默认值=%v", cfg.Server.WriteTimeout)
	}
}

func TestLoadMissingModelName(t *testing.T) {
	clearEnv(t)
	t.Setenv("MODEL_API_KEY", "sk-test")

	if _, err := Load(); err == nil {
		t.Fatal("缺少 MODEL_NAME 时应报错")
	}
}

func TestLoadMissingCredentials(t *testing.T) {
	clearEnv(t)
	t.Setenv("MODEL_NAME", "ep-test")

	if _, err := Load(); err == nil {
		t.Fatal("没有任何凭据时应报错")
	}
}

// AccessKey 与 SecretKey 成对出现时，可以替代 MODEL_API_KEY。
func TestLoadAccessKeyPair(t *testing.T) {
	clearEnv(t)
	t.Setenv("MODEL_NAME", "ep-test")
	t.Setenv("MODEL_ACCESS_KEY", "ak")
	t.Setenv("MODEL_SECRET_KEY", "sk")

	if _, err := Load(); err != nil {
		t.Fatalf("AK/SK 成对时应通过校验，得到: %v", err)
	}
}

func TestLoadBadPort(t *testing.T) {
	minimalEnv(t)
	t.Setenv("SERVER_PORT", "70000")

	if _, err := Load(); err == nil {
		t.Fatal("SERVER_PORT 超出范围时应报错")
	}
}

func TestLoadAgentMaxStepWhenEnabled(t *testing.T) {
	minimalEnv(t)
	t.Setenv("AGENT_ENABLED", "true")
	t.Setenv("AGENT_MAX_STEP", "0")

	if _, err := Load(); err == nil {
		t.Fatal("AGENT_ENABLED=true 且 AGENT_MAX_STEP<=0 时应报错")
	}
}

func TestEnvParsers(t *testing.T) {
	cases := []struct {
		name string
		val  string
		ok   func() bool
	}{
		{"bool", "true", func() bool { return envBool("K", false) == true }},
		{"bool_default_true", "false", func() bool { return envBool("K", true) == false }},
		{"int", "42", func() bool { return envInt("K", 1) == 42 }},
		{"duration", "3s", func() bool { return envDuration("K", time.Second) == 3*time.Second }},
		{"int_ptr", "7", func() bool { p := envIntPtr("K"); return p != nil && *p == 7 }},
		{"float_ptr", "0.25", func() bool { p := envFloatPtr("K"); return p != nil && *p == 0.25 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("K", c.val)
			if !c.ok() {
				t.Errorf("解析 %q 的结果不符合预期", c.val)
			}
		})
	}
}

// 非法数字不应让进程 panic，静默回落默认值即可。
func TestEnvParsersFallback(t *testing.T) {
	t.Setenv("K", "not-a-number")

	if v := envInt("K", 5); v != 5 {
		t.Errorf("envInt 非法值应回落默认 5，得到 %d", v)
	}
	if v := envBool("K", true); !v {
		t.Error("envBool 非法值应回落默认 true")
	}
	if v := envDuration("K", time.Second); v != time.Second {
		t.Errorf("envDuration 非法值应回落默认 1s，得到 %v", v)
	}
	if p := envIntPtr("K"); p != nil {
		t.Error("envIntPtr 非法值应返回 nil")
	}
}
