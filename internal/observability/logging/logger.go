package logging

import (
	"log/slog"
	"os"
	"strings"

	"agent-runtime/internal/config"
)

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
