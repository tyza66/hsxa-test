package engine

import (
	"context"
	"log/slog"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
)

// newTraceHandler 把 eino 组件的调用生命周期接到 slog 上，
// 用来观察模型请求量、token 消耗与失败原因。
//
// 这里只注册了关心的时机，未被注册的时机由 HandlerBuilder 内部通过
// TimingChecker 直接跳过，流式输入输出不会被强制关闭。
func newTraceHandler(logger *slog.Logger) callbacks.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	builder := callbacks.NewHandlerBuilder()

	builder.OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		mi := model.ConvCallbackInput(input)
		if mi == nil || len(mi.Messages) == 0 {
			return ctx
		}
		logger.DebugContext(ctx, "eino 组件调用开始",
			"component", info.Component,
			"name", info.Name,
			"messages", len(mi.Messages),
		)
		return ctx
	})

	builder.OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		mo := model.ConvCallbackOutput(output)
		if mo == nil || mo.Message == nil {
			return ctx
		}
		attrs := []any{"component", info.Component, "name", info.Name}
		if meta := mo.Message.ResponseMeta; meta != nil && meta.Usage != nil {
			attrs = append(attrs,
				"prompt_tokens", meta.Usage.PromptTokens,
				"completion_tokens", meta.Usage.CompletionTokens,
				"total_tokens", meta.Usage.TotalTokens,
				"finish_reason", meta.FinishReason,
			)
		}
		logger.DebugContext(ctx, "eino 组件调用结束", attrs...)
		return ctx
	})

	builder.OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
		logger.ErrorContext(ctx, "eino 组件调用失败",
			"component", info.Component,
			"name", info.Name,
			"error", err,
		)
		return ctx
	})

	return builder.Build()
}
