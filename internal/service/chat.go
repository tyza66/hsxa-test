// Package service 把传输层的请求体转换成 eino 调用，并做入参校验。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/tyza66/hsxa-test/internal/engine"
)

// ErrNoMessages 请求体缺少 messages 时返回。
var ErrNoMessages = errors.New("messages 不能为空")

// ChatRequest 是对话接口的请求体。
//
// messages 直接沿用 eino 的 schema.Message 结构，因此可以携带 role、content
// 以及多轮历史；目前只用到文本内容。
type ChatRequest struct {
	Messages []schema.Message `json:"messages"`
}

// ToolCall 是模型发起的工具调用。
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage 记录一次调用的 token 消耗。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// MessageView 是返回给调用方的消息视图。
// 不直接把 schema.Message 抛出去，避免调用方依赖 eino 的内部字段。
type MessageView struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     *Usage     `json:"usage,omitempty"`
}

// ToolSummary 用于工具列表接口。
type ToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ChatService 是对外提供的对话能力。
type ChatService struct {
	engine *engine.Engine
}

// NewChatService 创建对话服务。
func NewChatService(e *engine.Engine) *ChatService {
	return &ChatService{engine: e}
}

// Generate 一次性拿到完整回复。
func (s *ChatService) Generate(ctx context.Context, req *ChatRequest) (*MessageView, error) {
	messages, err := normalizeMessages(req)
	if err != nil {
		return nil, err
	}

	msg, err := s.engine.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("生成回复失败: %w", err)
	}
	return toMessageView(msg), nil
}

// Stream 以流式方式拿到回复增量，返回的 StreamReader 由调用方关闭。
func (s *ChatService) Stream(ctx context.Context, req *ChatRequest) (*schema.StreamReader[*schema.Message], error) {
	messages, err := normalizeMessages(req)
	if err != nil {
		return nil, err
	}

	reader, err := s.engine.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("开启流式生成失败: %w", err)
	}
	return reader, nil
}

// Tools 列出当前引擎可调用的工具。
func (s *ChatService) Tools(ctx context.Context) ([]ToolSummary, error) {
	summaries := make([]ToolSummary, 0, len(s.engine.Tools()))
	for _, t := range s.engine.Tools() {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("读取工具信息失败: %w", err)
		}
		summaries = append(summaries, ToolSummary{
			Name:        info.Name,
			Description: info.Desc,
		})
	}
	return summaries, nil
}

// AgentEnabled 报告当前请求是否走智能体链路。
func (s *ChatService) AgentEnabled() bool {
	return s.engine.HasAgent()
}

// normalizeMessages 校验并拷贝请求中的历史消息，
// 避免调用方后续改动影响到引擎内部状态。
func normalizeMessages(req *ChatRequest) ([]*schema.Message, error) {
	if req == nil || len(req.Messages) == 0 {
		return nil, ErrNoMessages
	}

	messages := make([]*schema.Message, 0, len(req.Messages))
	for i := range req.Messages {
		m := req.Messages[i]
		if strings.TrimSpace(m.Content) == "" &&
			len(m.MultiContent) == 0 &&
			len(m.UserInputMultiContent) == 0 {
			return nil, fmt.Errorf("messages[%d] 内容为空", i)
		}
		messages = append(messages, &m)
	}
	return messages, nil
}

// toMessageView 把 eino 的消息转成对外的视图。
func toMessageView(msg *schema.Message) *MessageView {
	if msg == nil {
		return nil
	}

	view := &MessageView{
		Role:    string(msg.Role),
		Content: msg.Content,
	}
	for _, tc := range msg.ToolCalls {
		view.ToolCalls = append(view.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	if meta := msg.ResponseMeta; meta != nil && meta.Usage != nil {
		view.Usage = &Usage{
			PromptTokens:     meta.Usage.PromptTokens,
			CompletionTokens: meta.Usage.CompletionTokens,
			TotalTokens:      meta.Usage.TotalTokens,
		}
	}
	return view
}

// UsageFromUsage 从 eino 的 TokenUsage 构造对外的 Usage。
func UsageFromUsage(u schema.TokenUsage) *Usage {
	return &Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}
