package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"stableagent/internal/config"
	"stableagent/internal/infra/postgres"
	redisinfra "stableagent/internal/infra/redis"
	"stableagent/internal/security/authn"
	httpserver "stableagent/internal/transport/http"
	"stableagent/internal/usecase/eventapi"
	"stableagent/internal/usecase/taskapi"
)

// RegisterRoutes 注册 API Service 路由并初始化任务用例依赖。
func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	pool, err := postgres.NewPool(context.Background(), postgres.PoolConfig{
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 API PostgreSQL 连接池失败: %w", err)
	}

	eventHub := eventapi.NewHub(eventapi.DefaultHubBuffer)
	eventBus := redisinfra.NewEventBus(cfg.RedisAddr, logger)
	cancelStore := redisinfra.NewCancelStore(cfg.RedisAddr)
	eventService := eventapi.NewService(pool, eventHub, eventBus, logger)
	taskService := taskapi.NewServiceWithEventNotifierAndCancelStore(pool, eventService, cancelStore)

	eventBusCtx, cancelEventBus := context.WithCancel(context.Background())
	if eventBus != nil {
		go func() {
			if err := eventBus.Subscribe(eventBusCtx, eventService.PublishLocal); err != nil {
				logger.Warn("Redis AgentEvent 订阅退出", "error", err)
			}
		}()
	}

	registerPublicRoutes(router, cfg)
	registerTaskRoutes(router, taskService, eventService, cfg.JWTSecret, logger)
	registerWebRoutes(router, cfg, logger)

	logger.Info("api-service 路由注册完成")
	// 返回 cleanup 交给服务启动器在优雅关闭阶段释放连接池。
	return func(context.Context) error {
		cancelEventBus()
		if eventBus != nil {
			_ = eventBus.Close()
		}
		if cancelStore != nil {
			_ = cancelStore.Close()
		}
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
func registerTaskRoutes(router chi.Router, service taskService, events eventService, jwtSecret string, logger *slog.Logger) {
	handler := newTaskHandler(service, logger)
	eventHandler := newEventHandler(events, logger)
	jwtMiddleware := authn.JWTMiddleware(authn.JWTMiddlewareConfig{Secret: jwtSecret})

	router.Group(func(r chi.Router) {
		r.Use(jwtMiddleware)
		r.Post("/api/v1/tasks", handler.createTask)
		r.Get("/api/v1/tasks", handler.listTasks)
		r.Get("/api/v1/tasks/{task_id}", handler.getTask)
		r.Post("/api/v1/tasks/{task_id}/cancel", handler.cancelTask)
		r.Post("/api/v1/tasks/{task_id}/resume", handler.resumeTask)
		r.Get("/api/v1/tasks/{task_id}/events", eventHandler.listTaskEvents)
		r.Get("/api/v1/tasks/{task_id}/events/stream", eventHandler.streamTaskEvents)
		r.Get("/api/v1/tasks/{task_id}/tool-calls", handler.listToolCalls)
		r.Get("/api/v1/tasks/{task_id}/checkpoints", handler.listCheckpoints)
		r.Get("/api/v1/tasks/{task_id}/artifacts", handler.listArtifacts)
		r.Post("/api/v1/tool-calls/{call_id}/approve", func(w http.ResponseWriter, r *http.Request) {
			handler.decideToolCall(w, r, true)
		})
		r.Post("/api/v1/tool-calls/{call_id}/reject", func(w http.ResponseWriter, r *http.Request) {
			handler.decideToolCall(w, r, false)
		})
	})
}
