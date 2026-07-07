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
	if err := bootstrap.RunHTTPService("llm-gateway", ":8083", registerRoutes); err != nil {
		slog.Error("llm-gateway 异常退出", "error", err)
		os.Exit(1)
	}
}

func registerRoutes(mux *http.ServeMux, cfg config.Config, logger *slog.Logger) {
	mux.HandleFunc("/llm/v1/status", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "LLM 统一入口，后续负责模型路由、token 统计、限流、重试和审计",
			"status":  "ready",
		})
	})

	logger.Info("llm-gateway 路由注册完成")
}
