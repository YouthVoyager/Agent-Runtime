package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/bootstrap"
	"agent-runtime/internal/config"
	"agent-runtime/internal/httpserver"
)

func main() {
	if err := bootstrap.RunHTTPService("tool-gateway", ":8082", registerRoutes); err != nil {
		slog.Error("tool-gateway 异常退出", "error", err)
		os.Exit(1)
	}
}

func registerRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) {
	router.Get("/tools/v1/catalog", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"tools": []map[string]string{
				{"name": "read_document", "risk": "LOW"},
				{"name": "write_artifact", "risk": "MEDIUM"},
				{"name": "send_external_message", "risk": "HIGH"},
				{"name": "dangerous_admin_action", "risk": "CRITICAL"},
			},
		})
	})

	logger.Info("tool-gateway 路由注册完成")
}
