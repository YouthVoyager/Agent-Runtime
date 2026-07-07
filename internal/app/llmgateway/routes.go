package llmgateway

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
	httpserver "agent-runtime/internal/transport/http"
)

func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	router.Get("/llm/v1/status", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "LLM 统一入口，后续负责模型路由、token 统计、限流、重试和审计",
			"status":  "ready",
		})
	})

	logger.Info("llm-gateway 路由注册完成")
	return nil, nil
}
