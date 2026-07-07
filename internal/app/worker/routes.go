package worker

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
	httpserver "agent-runtime/internal/transport/http"
)

func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	router.Get("/runtime/v1/status", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "Agent Runtime 后台执行进程，后续负责 step、checkpoint、cancel 和 resume",
			"worker":  "idle",
		})
	})

	logger.Info("runtime-worker 路由注册完成")
	return nil, nil
}
