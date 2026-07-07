package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
	"agent-runtime/internal/infra/postgres"
	"agent-runtime/internal/security/authn"
	httpserver "agent-runtime/internal/transport/http"
	"agent-runtime/internal/usecase/taskapi"
)

// RegisterRoutes 注册 API Service 路由并初始化任务用例依赖。
func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	pool, err := postgres.NewPool(context.Background(), postgres.PoolConfig{
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 API PostgreSQL 连接池失败: %w", err)
	}

	taskService := taskapi.NewService(pool)
	registerPublicRoutes(router, cfg)
	registerTaskRoutes(router, taskService, cfg.JWTSecret, logger)

	logger.Info("api-service 路由注册完成")
	// 返回 cleanup 交给服务启动器在优雅关闭阶段释放连接池。
	return func(context.Context) error {
		pool.Close()
		return nil
	}, nil
}

// registerPublicRoutes 注册无需鉴权的 API Service 公共路由。
func registerPublicRoutes(router chi.Router, cfg config.Config) {
	router.Get("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/healthz", http.StatusTemporaryRedirect)
	})

	router.Get("/api/v1/service-info", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteData(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "用户 API，承载任务创建、查询、审批、取消和恢复",
		})
	})
}

// registerTaskRoutes 注册任务 API，并为任务路由统一挂载 JWT 鉴权。
func registerTaskRoutes(router chi.Router, service taskService, jwtSecret string, logger *slog.Logger) {
	handler := newTaskHandler(service, logger)
	jwtMiddleware := authn.JWTMiddleware(authn.JWTMiddlewareConfig{Secret: jwtSecret})

	router.Group(func(r chi.Router) {
		r.Use(jwtMiddleware)
		r.Post("/api/v1/tasks", handler.createTask)
		r.Get("/api/v1/tasks", handler.listTasks)
		r.Get("/api/v1/tasks/{task_id}", handler.getTask)
	})
}
