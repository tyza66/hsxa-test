// Package engine 把 eino 的各个部件（ChatModel、工具、回调、ReAct 智能体）
// 组装成上层可以直接调用的对话引擎。
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/tyza66/hsxa-test/internal/config"
)

// Engine 是服务的对话引擎，内部持有 eino 的模型、工具与智能体实例。
type Engine struct {
	cfg    *config.Config
	logger *slog.Logger

	base  model.ToolCallingChatModel // 未绑定工具的原始模型，智能体自己会去绑定
	chat  model.ToolCallingChatModel // 已绑定工具的模型，用于不走智能体的直连场景
	tools []tool.BaseTool
	agent *react.Agent // AGENT_ENABLED=true 时才非空
}

// New 构建引擎：注册回调、创建 ChatModel、装配工具，并按需构建 ReAct 智能体。
func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Engine, error) {
	if cfg == nil {
		return nil, errors.New("engine: 配置为 nil")
	}
	if logger == nil {
		logger = slog.Default()
	}

	// 全局注册一次，之后所有 eino 组件的生命周期都会进到同一套日志里。
	callbacks.InitCallbackHandlers([]callbacks.Handler{newTraceHandler(logger)})

	base, err := newChatModel(ctx, &cfg.Model)
	if err != nil {
		return nil, err
	}

	tools, err := newTools()
	if err != nil {
		return nil, err
	}

	infos, err := toolInfos(tools)
	if err != nil {
		return nil, err
	}

	e := &Engine{
		cfg:    cfg,
		logger: logger,
		base:   base,
		tools:  tools,
	}

	// WithTools 返回新实例而不原地修改 base，所以 base 始终是干净的。
	e.chat, err = base.WithTools(infos)
	if err != nil {
		return nil, fmt.Errorf("engine: 绑定工具失败: %w", err)
	}

	if cfg.Agent.Enabled {
		agent, err := e.newAgent(ctx)
		if err != nil {
			return nil, err
		}
		e.agent = agent
		logger.InfoContext(ctx, "ReAct 智能体已启用",
			"tools", len(tools),
			"max_step", cfg.Agent.MaxStep,
		)
	} else {
		logger.InfoContext(ctx, "ReAct 智能体未启用，请求将直接到达 ChatModel",
			"tools", len(tools),
		)
	}

	return e, nil
}

// ChatModel 返回已绑定工具、可直接使用的 ChatModel。
func (e *Engine) ChatModel() model.ToolCallingChatModel {
	return e.chat
}

// Tools 返回引擎暴露给模型的工具集合。
func (e *Engine) Tools() []tool.BaseTool {
	return e.tools
}

// HasAgent 报告 ReAct 智能体是否处于启用状态。
func (e *Engine) HasAgent() bool {
	return e.agent != nil
}

// Generate 非流式生成，启用智能体时走智能体链路，否则直接调用 ChatModel。
func (e *Engine) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if e.agent != nil {
		// react.Agent 只认 agent.AgentOption，模型侧参数需要显式包一层
		// 才能透传到智能体内部的 ChatModel 节点。
		return e.agent.Generate(ctx, messages, react.WithChatModelOptions(opts...))
	}
	return e.chat.Generate(ctx, messages, opts...)
}

// Stream 流式生成，返回的 StreamReader 由调用方负责关闭。
func (e *Engine) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if e.agent != nil {
		// 同 Generate：模型侧参数经 WithChatModelOptions 转换为智能体选项。
		return e.agent.Stream(ctx, messages, react.WithChatModelOptions(opts...))
	}
	return e.chat.Stream(ctx, messages, opts...)
}
