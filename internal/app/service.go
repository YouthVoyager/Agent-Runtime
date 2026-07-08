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

	"stableagent/internal/config"
	"stableagent/internal/observability/logging"
	"stableagent/internal/observability/metrics"
	"stableagent/internal/observability/tracing"
	httpserver "stableagent/internal/transport/http"
	apperrors "stableagent/pkg/errors"
)

type RegisterRoutes func(router chi.Router, cfg config.Config, logger *slog.Logger) (func(context.Context) error, error)

// RunHTTPService 完成 HTTP 服务的配置加载、公共 middleware 注册、路由注册和生命周期管理。
func RunHTTPService(serviceName string, defaultAddr string, register RegisterRoutes) error {
	cfg, err := config.Load(serviceName, config.Defaults{HTTPAddr: defaultAddr})
	if err != nil {
		return err
	}

	logger := logging.New(cfg)
	startedAt := time.Now()

	// OTel 追踪初始化;未配置 OTEL_EXPORTER_OTLP_ENDPOINT 时保持 no-op。
	traceShutdown, err := tracing.Setup(context.Background(), cfg.ServiceName, cfg.Env)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = traceShutdown(shutdownCtx)
	}()

	router := chi.NewRouter()
	router.Use(
		httpserver.WithRecovery(logger),
		httpserver.WithRequestID(cfg.RequestIDHeader),
		httpserver.WithTracing(cfg.ServiceName),
		httpserver.WithMetrics(cfg.ServiceName),
		httpserver.WithAccessLog(logger),
	)
	// 未匹配路由和方法不支持也走统一错误结构，避免框架默认纯文本响应泄漏到 API。
	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeNotFound, "接口不存在"))
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpserver.WriteError(w, r, apperrors.New(apperrors.CodeMethodNotAllowed, "请求方法不支持"))
	})

	httpserver.RegisterHealthRoutes(router, cfg, startedAt)
	// /metrics 使用 prometheus/client_golang,包含 Go runtime、HTTP、任务与工具调用指标。
	metrics.ServiceUp.WithLabelValues(cfg.ServiceName, cfg.Env).Set(1)
	router.Method(http.MethodGet, "/metrics", metrics.Handler())
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
		// 业务资源清理与 HTTP 关闭分离，确保数据库连接池等资源有独立超时时间。
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := cleanup(cleanupCtx); err != nil && runErr == nil {
			return err
		}
	}
	return runErr
}
