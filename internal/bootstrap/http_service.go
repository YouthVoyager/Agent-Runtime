package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent-runtime/internal/config"
	"agent-runtime/internal/httpserver"
	"agent-runtime/internal/logging"
)

type RegisterRoutes func(mux *http.ServeMux, cfg config.Config, logger *slog.Logger)

func RunHTTPService(serviceName string, defaultAddr string, register RegisterRoutes) error {
	cfg, err := config.Load(serviceName, config.Defaults{HTTPAddr: defaultAddr})
	if err != nil {
		return err
	}

	logger := logging.New(cfg)
	startedAt := time.Now()
	mux := http.NewServeMux()
	httpserver.RegisterHealthRoutes(mux, cfg, startedAt)
	if register != nil {
		register(mux, cfg, logger)
	}

	handler := httpserver.Chain(
		mux,
		httpserver.WithRecovery(logger),
		httpserver.WithRequestID(cfg.RequestIDHeader),
		httpserver.WithAccessLog(logger),
	)

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

	return httpserver.Run(ctx, cfg, logger, handler)
}
