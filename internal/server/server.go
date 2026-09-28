// Package server 提供基于 net/http 的 HTTP 接口层。
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/tyza66/hsxa-test/internal/service"
)

// Config 描述 HTTP 服务参数。
type Config struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Server 组合 http.Server 与业务服务。
type Server struct {
	cfg     Config
	chat    *service.ChatService
	fofa    *service.FofaService // 非 nil 时才注册 FOFA 路由
	logger  *slog.Logger
	httpSrv *http.Server
}

// New 构造 HTTP 服务并注册路由。
//
// fofa 传 nil 表示 FOFA 链路未装载，此时 /v1/fofa 不会注册。
func New(cfg Config, chat *service.ChatService, fofa *service.FofaService, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{cfg: cfg, chat: chat, fofa: fofa, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/tools", s.handleTools)
	mux.HandleFunc("POST /v1/chat", s.handleChat)
	mux.HandleFunc("POST /v1/chat/stream", s.handleChatStream)
	if fofa != nil {
		mux.HandleFunc("POST /v1/fofa", s.handleFofa)
	}

	s.httpSrv = &http.Server{
		Addr:         cfg.Addr,
		Handler:      loggingMiddleware(mux, logger),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	return s
}

// Run 启动服务，并在 ctx 取消时优雅停机。
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.InfoContext(ctx, "http 服务已启动",
			"addr", s.cfg.Addr,
			"agent", s.chat.AgentEnabled(),
		)
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http 服务异常退出: %w", err)
		}
		return nil
	case <-ctx.Done():
		s.logger.InfoContext(ctx, "收到退出信号，开始优雅停机")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()
	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅停机失败: %w", err)
	}
	return nil
}

// loggingMiddleware 记录请求的方法、路径、状态码与耗时。
func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.InfoContext(r.Context(), "http 请求",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// statusRecorder 在不影响写出的前提下记录状态码。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush 让包装后的 ResponseWriter 仍然支持流式下发。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
