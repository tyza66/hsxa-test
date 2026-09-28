// Command hsxa 是基于字节跳动 eino 的对话引擎可执行入口。
//
// 支持三个子命令：
//
//	hsxa serve    启动 HTTP 服务（默认命令）
//	hsxa chat     在终端里和引擎流式对话
//	hsxa tools    列出引擎可调用的工具
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cloudwego/eino/schema"

	"github.com/tyza66/hsxa-test/internal/config"
	"github.com/tyza66/hsxa-test/internal/engine"
	"github.com/tyza66/hsxa-test/internal/server"
	"github.com/tyza66/hsxa-test/internal/service"
)

const appName = "hsxa"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", appName, err)
		os.Exit(1)
	}
}

// run 解析子命令并派发到对应实现。
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := "serve"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		command = strings.TrimSpace(args[0])
	}

	logger := newLogger(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT"), stderr)

	switch command {
	case "serve":
		return serveCmd(context.Background(), logger)
	case "chat":
		return chatCmd(context.Background(), stdin, stdout, logger)
	case "tools":
		return toolsCmd(context.Background(), stdout, logger)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("未知命令 %q", command)
	}
}

// newLogger 按 LOG_FORMAT / LOG_LEVEL 初始化 slog。
func newLogger(level, format string, w io.Writer) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}
	return slog.New(handler)
}

// serveCmd 启动 HTTP 服务直至收到退出信号。
func serveCmd(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	chat, err := buildChatService(ctx, cfg, logger)
	if err != nil {
		return err
	}

	srv := server.New(server.Config{
		Addr:            cfg.Server.Addr(),
		ReadTimeout:     cfg.Server.ReadTimeout,
		WriteTimeout:    cfg.Server.WriteTimeout,
		ShutdownTimeout: cfg.Server.ShutdownTimeout,
	}, chat, logger)

	// SIGINT / SIGTERM 触发优雅停机。
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
}

// chatCmd 在终端与引擎做单轮流式对话，仅用于本地调试。
func chatCmd(ctx context.Context, stdin io.Reader, stdout io.Writer, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	chat, err := buildChatService(ctx, cfg, logger)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "hsxa chat（agent=%v，输入 exit 或 quit 退出）\n", chat.AgentEnabled())

	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Fprint(stdout, "> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return nil
		}
		if err := ask(ctx, chat, line, stdout); err != nil {
			fmt.Fprintf(stdout, "\n[error] %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取输入失败: %w", err)
	}
	return nil
}

// ask 发起一轮流式请求并增量打印回复。
func ask(ctx context.Context, chat *service.ChatService, prompt string, out io.Writer) error {
	reader, err := chat.Stream(ctx, &service.ChatRequest{
		Messages: []schema.Message{{Role: schema.User, Content: prompt}},
	})
	if err != nil {
		return err
	}
	defer reader.Close()

	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if chunk == nil {
			continue
		}

		for _, tc := range chunk.ToolCalls {
			if tc.Function.Name == "" && tc.Arguments == "" {
				continue
			}
			fmt.Fprintf(out, "\n[tool] %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
		}
		if chunk.Content != "" {
			fmt.Fprint(out, chunk.Content)
		}
	}

	fmt.Fprintln(out)
	return nil
}

// toolsCmd 打印引擎可调用的工具列表。
func toolsCmd(ctx context.Context, stdout io.Writer, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	chat, err := buildChatService(ctx, cfg, logger)
	if err != nil {
		return err
	}

	tools, err := chat.Tools(ctx)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(tools, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, string(data))
	return nil
}

// buildChatService 组装引擎与对话服务。
func buildChatService(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*service.ChatService, error) {
	eng, err := engine.New(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	return service.NewChatService(eng), nil
}

// usage 打印命令帮助。
func usage(w io.Writer) {
	fmt.Fprintf(w, `%s - 基于字节跳动 eino 的对话引擎

Usage:
  %s <command>

Commands:
  serve    启动 HTTP 服务（默认）
  chat     在终端与引擎流式对话
  tools    列出引擎可调用的工具
  help     显示本帮助

Environment:
  必填：MODEL_API_KEY、MODEL_NAME
  可选：MODEL_PROVIDER、MODEL_BASE_URL、MODEL_REGION、AGENT_ENABLED 等
  完整清单见仓库根目录的 .env.example
`, appName, appName)
}
