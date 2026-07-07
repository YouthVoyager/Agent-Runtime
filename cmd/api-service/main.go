package main

import (
	"log/slog"
	"net/http"
	"os"

	"agent-runtime/internal/apperrors"
	"agent-runtime/internal/bootstrap"
	"agent-runtime/internal/config"
	"agent-runtime/internal/httpserver"
)

func main() {
	if err := bootstrap.RunHTTPService("api-service", ":8080", registerRoutes); err != nil {
		slog.Error("api-service 异常退出", "error", err)
		os.Exit(1)
	}
}

func registerRoutes(mux *http.ServeMux, cfg config.Config, logger *slog.Logger) {
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/healthz", http.StatusTemporaryRedirect)
	})

	mux.HandleFunc("/api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			apperrors.WriteJSON(w, apperrors.New(apperrors.CodeNotImplemented, "第 1 周仅完成服务骨架，任务 API 将在后续迭代实现"))
			return
		}
		apperrors.WriteJSON(w, apperrors.New(apperrors.CodeNotImplemented, "第 1 周仅完成服务骨架，任务创建将在第 3 周实现"))
	})

	mux.HandleFunc("/api/v1/service-info", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "用户 API，后续承载任务创建、查询、审批、取消和恢复",
		})
	})

	logger.Info("api-service 路由注册完成")
}
