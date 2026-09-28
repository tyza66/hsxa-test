package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino-ext/components/model/ark"

	"github.com/tyza66/hsxa-test/internal/config"
)

// newChatModel 按配置创建对应的 eino ChatModel 实现。
func newChatModel(ctx context.Context, cfg *config.ModelConfig) (model.ToolCallingChatModel, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case config.ProviderArk:
		return newArkChatModel(ctx, cfg)
	default:
		return nil, fmt.Errorf("engine: 不支持的模型供应商 %q", cfg.Provider)
	}
}

// newArkChatModel 创建火山方舟 ChatModel。
//
// 空值的 BaseURL / Region 由 ark 组件回落到默认值，因此这里原样透传即可。
func newArkChatModel(ctx context.Context, cfg *config.ModelConfig) (model.ToolCallingChatModel, error) {
	arkCfg := ark.ChatModelConfig{
		APIKey:      cfg.APIKey,
		AccessKey:   cfg.AccessKey,
		SecretKey:   cfg.SecretKey,
		Model:       cfg.Name,
		BaseURL:     cfg.BaseURL,
		Region:      cfg.Region,
		Temperature: cfg.Temperature,
		TopP:        cfg.TopP,
		MaxTokens:   cfg.MaxTokens,
		RetryTimes:  cfg.RetryTimes,
	}
	if cfg.Timeout > 0 {
		timeout := cfg.Timeout
		arkCfg.Timeout = &timeout
	}

	cm, err := ark.NewChatModel(ctx, &arkCfg)
	if err != nil {
		return nil, fmt.Errorf("engine: 初始化 ark chat model 失败: %w", err)
	}
	return cm, nil
}
