package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
	httpserver "agent-runtime/internal/transport/http"
	apperrors "agent-runtime/pkg/errors"
)

func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) {
	router.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/healthz", http.StatusTemporaryRedirect)
	})

	router.Post("/api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		apperrors.WriteJSON(w, apperrors.New(apperrors.CodeNotImplemented, "第 1 周仅完成服务骨架，任务创建将在第 3 周实现"))
	})

	router.Get("/api/v1/service-info", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "用户 API，后续承载任务创建、查询、审批、取消和恢复",
		})
	})

	logger.Info("api-service 路由注册完成")
}
