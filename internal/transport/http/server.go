package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"stableagent/internal/config"
)

// Run 启动 HTTP 服务，并在上下文取消时执行优雅关闭。
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger, handler http.Handler) error {
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("HTTP 服务启动", "addr", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		// 优雅关闭使用独立 context，避免外层取消后直接中断正在处理的请求。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		logger.Info("开始优雅关闭 HTTP 服务", "timeout", cfg.ShutdownTimeout.String())
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		logger.Info("HTTP 服务已关闭")
		return nil
	}
}
