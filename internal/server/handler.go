package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/tyza66/hsxa-test/internal/service"
)

// maxBodyBytes 限制请求体大小，防止异常大的 body 打满内存。
const maxBodyBytes = 1 << 20

// sseEvent 是流式响应中每条 SSE 事件的载荷。
type sseEvent struct {
	Type     string            `json:"type"` // delta | tool_call | error | done
	Content  string            `json:"content,omitempty"`
	ToolCall *service.ToolCall `json:"tool_call,omitempty"`
	Usage    *service.Usage    `json:"usage,omitempty"`
}

// writeJSON 写出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

// writeError 统一错误输出。
func writeError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		err = errors.New("未知错误")
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// decodeJSON 解析请求体，超长时由 MaxBytesReader 通过 w 终止连接。
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		return fmt.Errorf("请求体解析失败: %w", err)
	}
	return nil
}

// handleHealth 存活探针，顺带暴露当前是否启用了智能体。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"agent":  s.chat.AgentEnabled(),
		"fofa":   s.fofa != nil,
	})
}

// handleTools 列出可用工具。
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	tools, err := s.chat.Tools(r.Context())
	if err != nil {
		s.logger.ErrorContext(r.Context(), "读取工具列表失败", "error", err)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

// handleFofa 跑 FOFA 链路，把自然语言需求转成结构化答案。
//
// 入参缺问题是调用方错误（400），链路自身失败是上游问题（502）。
func (s *Server) handleFofa(w http.ResponseWriter, r *http.Request) {
	if s.fofa == nil {
		writeError(w, http.StatusNotFound, errors.New("FOFA 链路未装载"))
		return
	}

	var req service.FofaRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	answer, err := s.fofa.Convert(r.Context(), &req)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, service.ErrEmptyQuestion) {
			status = http.StatusBadRequest
		}
		s.logger.ErrorContext(r.Context(), "FOFA 链路执行失败", "error", err)
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

// handleChat 非流式对话。
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req service.ChatRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	msg, err := s.chat.Generate(r.Context(), &req)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "生成回复失败", "error", err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

// handleChatStream 以 SSE 下发流式回复。
//
// 事件类型：delta（文本增量）、tool_call（模型发起工具调用）、
// error（出错）与 done（结束，携带 token 用量）。
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("response writer 不支持流式输出"))
		return
	}

	var req service.ChatRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")

	reader, err := s.chat.Stream(r.Context(), &req)
	if err != nil {
		s.logger.ErrorContext(r.Context(), "开启流式生成失败", "error", err)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer reader.Close()

	var usage *service.Usage
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.logger.ErrorContext(r.Context(), "读取流式增量失败", "error", err)
			s.writeSSE(w, flusher, sseEvent{Type: "error", Content: err.Error()})
			return
		}
		if chunk == nil {
			continue
		}

		// 流式响应里只有最后一个 chunk 通常带 usage，取到就不再覆盖。
		if meta := chunk.ResponseMeta; meta != nil && meta.Usage != nil && usage == nil {
			usage = service.UsageFromUsage(*meta.Usage)
		}

		for _, tc := range chunk.ToolCalls {
			if tc.Function.Name == "" && tc.ID == "" {
				continue
			}
			s.writeSSE(w, flusher, sseEvent{
				Type: "tool_call",
				ToolCall: &service.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}

		if chunk.Content != "" {
			s.writeSSE(w, flusher, sseEvent{Type: "delta", Content: chunk.Content})
		}
	}

	s.writeSSE(w, flusher, sseEvent{Type: "done", Usage: usage})
}

// writeSSE 写出一条 SSE 事件并立即刷给客户端。
func (s *Server) writeSSE(w http.ResponseWriter, flusher http.Flusher, event sseEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("序列化 SSE 事件失败", "error", err)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
	flusher.Flush()
}
