package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
	"agent-runtime/internal/observability/logging"
	httpserver "agent-runtime/internal/transport/http"
	apperrors "agent-runtime/pkg/errors"
)

type RegisterRoutes func(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error)

func RunHTTPService(serviceName string, defaultAddr string, register RegisterRoutes) error {
	cfg, err := config.Load(serviceName, config.Defaults{HTTPAddr: defaultAddr})
	if err != nil {
		return err
	}

	logger := logging.New(cfg)
	startedAt := time.Now()
	router := chi.NewRouter()
	router.Use(
		httpserver.WithRecovery(logger),
		httpserver.WithRequestID(cfg.RequestIDHeader),
		httpserver.WithAccessLog(logger),
	)
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeNotFound, "接口不存在"))
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeMethodNotAllowed, "请求方法不支持"))
	})

	httpserver.RegisterHealthRoutes(router, cfg, startedAt)
	var cleanup func(context.Context) error
	if register != nil {
		routeCleanup, err := register(router, cfg, logger)
		if err != nil {
			return err
		}
		cleanup = routeCleanup
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("服务配置加载完成",
		"http_addr", cfg.HTTPAddr,
		"database_configured", cfg.DatabaseURL != "",
		"redis_addr", cfg.RedisAddr,
		"temporal_address", cfg.TemporalAddress,
		"minio_endpoint", cfg.MinIOEndpoint,
		"jaeger_endpoint", cfg.JaegerEndpoint,
	)

	runErr := httpserver.Run(ctx, cfg, logger, router)
	if cleanup != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := cleanup(cleanupCtx); err != nil && runErr == nil {
			return err
		}
	}
	return runErr
}
