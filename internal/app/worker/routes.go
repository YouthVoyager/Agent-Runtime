package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"stableagent/internal/config"
	"stableagent/internal/infra/postgres"
	redisinfra "stableagent/internal/infra/redis"
	httpserver "stableagent/internal/transport/http"
	"stableagent/internal/usecase/eventapi"
	"stableagent/internal/usecase/runtimeworker"
)

// RegisterRoutes 注册 Runtime Worker 当前阶段的状态路由。
func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	pool, err := postgres.NewPool(context.Background(), postgres.PoolConfig{
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 Runtime Worker PostgreSQL 连接池失败: %w", err)
	}
	eventHub := eventapi.NewHub(eventapi.DefaultHubBuffer)
	eventBus := redisinfra.NewEventBus(cfg.RedisAddr, logger)
	eventService := eventapi.NewService(pool, eventHub, eventBus, logger)
	cancelStore := redisinfra.NewCancelStore(cfg.RedisAddr)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	runtimeService := runtimeworker.NewService(pool, eventService, cancelStore, runtimeworker.Config{
		LLMGatewayURL:  cfg.LLMGatewayURL,
		ToolGatewayURL: cfg.ToolGatewayURL,
		PollInterval:   cfg.WorkerPollInterval,
	}, logger)
	runtimeService.Start(workerCtx)

	router.Get("/runtime/v1/status", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteJSON(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"role":    "StableAgent 后台执行进程，负责 outbox、step、checkpoint、cancel 和 resume",
			"worker":  "running",
		})
	})

	logger.Info("runtime-worker 路由注册完成")
	return func(context.Context) error {
		cancelWorker()
		if cancelStore != nil {
			_ = cancelStore.Close()
		}
		if eventBus != nil {
			_ = eventBus.Close()
		}
		pool.Close()
		return nil
	}, nil
}
