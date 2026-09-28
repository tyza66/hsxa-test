// Package config 负责从环境变量加载并校验运行期配置。
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderArk 表示使用火山方舟（Volcano Engine Ark）作为模型供应商。
const ProviderArk = "ark"

// DefaultPersona 是未配置 AGENT_PERSONA 时使用的系统人设。
const DefaultPersona = "You are a helpful assistant. Reply in the language the user writes in."

// Config 聚合服务运行所需的全部配置。
type Config struct {
	Server ServerConfig
	Model  ModelConfig
	Agent  AgentConfig
}

// ServerConfig 描述 HTTP 监听参数。
type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Addr 返回监听地址。
func (c ServerConfig) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

// ModelConfig 描述转发给 eino ChatModel 实现的参数。
type ModelConfig struct {
	Provider    string
	Name        string
	APIKey      string
	AccessKey   string
	SecretKey   string
	BaseURL     string
	Region      string
	Temperature *float32
	TopP        *float32
	MaxTokens   *int
	RetryTimes  *int
	Timeout     time.Duration
}

// AgentConfig 控制包裹在 ChatModel 外的 ReAct 智能体。
type AgentConfig struct {
	Enabled bool
	MaxStep int
	Persona string
}

// Load 读取环境变量并做校验，返回可直接使用的配置。
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host:            envString("SERVER_HOST", "0.0.0.0"),
			Port:            envInt("SERVER_PORT", 8080),
			ReadTimeout:     envDuration("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    envDuration("SERVER_WRITE_TIMEOUT", 10*time.Minute),
			ShutdownTimeout: envDuration("SERVER_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Model: ModelConfig{
			Provider:    envString("MODEL_PROVIDER", ProviderArk),
			Name:        trimEnv("MODEL_NAME"),
			APIKey:      trimEnv("MODEL_API_KEY"),
			AccessKey:   trimEnv("MODEL_ACCESS_KEY"),
			SecretKey:   trimEnv("MODEL_SECRET_KEY"),
			BaseURL:     trimEnv("MODEL_BASE_URL"),
			Region:      trimEnv("MODEL_REGION"),
			Temperature: envFloatPtr("MODEL_TEMPERATURE"),
			TopP:        envFloatPtr("MODEL_TOP_P"),
			MaxTokens:   envIntPtr("MODEL_MAX_TOKENS"),
			RetryTimes:  envIntPtr("MODEL_RETRY_TIMES"),
			Timeout:     envDuration("MODEL_TIMEOUT", 10*time.Minute),
		},
		Agent: AgentConfig{
			Enabled: envBool("AGENT_ENABLED", false),
			MaxStep: envInt("AGENT_MAX_STEP", 12),
			Persona: envString("AGENT_PERSONA", DefaultPersona),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// validate 在启动前拦掉明显残缺的配置，避免服务跑起来之后才在请求上暴露问题。
func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("config: SERVER_PORT %d 超出范围（1-65535）", c.Server.Port)
	}
	if c.Server.ReadTimeout < 0 || c.Server.WriteTimeout < 0 {
		return errors.New("config: SERVER_READ_TIMEOUT / SERVER_WRITE_TIMEOUT 不能为负")
	}

	switch strings.ToLower(strings.TrimSpace(c.Model.Provider)) {
	case ProviderArk:
		if c.Model.Name == "" {
			return errors.New("config: MODEL_NAME 必填（Ark 接入点 / 模型 ID）")
		}
		if c.Model.APIKey == "" && (c.Model.AccessKey == "" || c.Model.SecretKey == "") {
			return errors.New("config: 需要 MODEL_API_KEY，或同时提供 MODEL_ACCESS_KEY 与 MODEL_SECRET_KEY")
		}
	default:
		return fmt.Errorf("config: 不支持的 MODEL_PROVIDER %q（当前支持 %q）", c.Model.Provider, ProviderArk)
	}

	if c.Agent.Enabled && c.Agent.MaxStep <= 0 {
		return fmt.Errorf("config: AGENT_MAX_STEP 必须大于 0，当前是 %d", c.Agent.MaxStep)
	}
	return nil
}

func trimEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envString(key, def string) string {
	if v := trimEnv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := envIntPtr(key); v != nil {
		return *v
	}
	return def
}

func envIntPtr(key string) *int {
	raw := trimEnv(key)
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &v
}

func envFloatPtr(key string) *float32 {
	raw := trimEnv(key)
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	f := float32(v)
	return &f
}

func envBool(key string, def bool) bool {
	raw := trimEnv(key)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	raw := trimEnv(key)
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return def
	}
	return v
}
