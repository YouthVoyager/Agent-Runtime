package toolgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"stableagent/internal/config"
	"stableagent/internal/infra/postgres"
	redisinfra "stableagent/internal/infra/redis"
	httpserver "stableagent/internal/transport/http"
	"stableagent/internal/usecase/eventapi"
	toolusecase "stableagent/internal/usecase/toolgateway"
	apperrors "stableagent/pkg/errors"
)

// RegisterRoutes 注册 Tool Gateway 当前阶段的工具目录路由。
func RegisterRoutes(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	pool, err := postgres.NewPool(context.Background(), postgres.PoolConfig{
		DatabaseURL: cfg.DatabaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 Tool Gateway PostgreSQL 连接池失败: %w", err)
	}
	eventHub := eventapi.NewHub(eventapi.DefaultHubBuffer)
	eventBus := redisinfra.NewEventBus(cfg.RedisAddr, logger)
	eventService := eventapi.NewService(pool, eventHub, eventBus, logger)
	service := toolusecase.NewService(pool, eventService)

	router.Get("/tools/v1/catalog", func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteData(w, http.StatusOK, map[string]any{
			"service": cfg.ServiceName,
			"tools":   service.Catalog(),
		})
	})
	router.Post("/tools/v1/calls", func(w http.ResponseWriter, r *http.Request) {
		var req toolusecase.CallInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpserver.WriteError(w, r, apperrors.Wrap(apperrors.CodeInvalidArg, "请求 JSON 格式错误", err))
			return
		}
		result, err := service.Call(r.Context(), req)
		if err != nil {
			httpserver.WriteError(w, r, err)
			return
		}
		httpserver.WriteData(w, http.StatusOK, result)
	})

	logger.Info("tool-gateway 路由注册完成")
	return func(context.Context) error {
		if eventBus != nil {
			_ = eventBus.Close()
		}
		pool.Close()
		return nil
	}, nil
}
