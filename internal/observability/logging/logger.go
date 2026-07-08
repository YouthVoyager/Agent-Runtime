package logging

import (
	"log/slog"
	"os"
	"strings"

	"stableagent/internal/config"
)

// New 创建带服务名和环境字段的 JSON 结构化日志器。
func New(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(cfg.LogLevel)) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(
		"service", cfg.ServiceName,
		"env", cfg.Env,
	)
}
