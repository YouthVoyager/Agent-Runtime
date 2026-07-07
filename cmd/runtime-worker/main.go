package main

import (
	"log/slog"
	"net/http"
	"os"

	"agent-runtime/internal/bootstrap"
	"agent-runtime/internal/config"
	"agent-runtime/internal/httpserver"
)

func main() {
	if err := bootstrap.RunHTTPService("runtime-worker", ":8081", registerRoutes); err != nil {
		slog.Error("runtime-worker 异常退出", "error", err)
		os.Exit(1)
	}
}

func registerRoutes(mux *http.ServeMux, cfg config.Config, logger *slog.Logger) {
	mux.HandleFunc("/runtime/v1/status", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "Agent Runtime 后台执行进程，后续负责 step、checkpoint、cancel 和 resume",
			"worker":  "idle",
		})
	})

	logger.Info("runtime-worker 路由注册完成")
}
