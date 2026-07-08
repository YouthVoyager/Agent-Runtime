package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"stableagent/pkg/errors"
)

type Middleware func(http.Handler) http.Handler

type contextKey string

const requestIDContextKey contextKey = "request_id"

// Chain 按声明顺序组合多个 HTTP middleware。
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

// WithRequestID 确保每个请求都有 request_id，并写入响应头和上下文。
func WithRequestID(headerName string) Middleware {
	if strings.TrimSpace(headerName) == "" {
		headerName = "X-Request-ID"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get(headerName))
			if requestID == "" {
				// 调用方未传入时由服务端生成，保证日志和错误响应始终可关联。
				requestID = newRequestID()
			}
			w.Header().Set(headerName, requestID)

			ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDFromContext 从上下文中读取 request_id。
func RequestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey).(string)
	return value
}

// WithRecovery 捕获 HTTP handler panic 并转换为统一错误响应。
func WithRecovery(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "HTTP 请求发生 panic",
						"request_id", RequestIDFromContext(r.Context()),
						"panic", recovered,
						"stack", string(debug.Stack()),
					)
					WriteError(w, r, apperrors.New(apperrors.CodeInternal, "系统内部错误"))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// WithAccessLog 记录每个 HTTP 请求的状态码和耗时。
func WithAccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(recorder, r)

			logger.InfoContext(r.Context(), "HTTP 请求完成",
				"request_id", RequestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.statusCode,
				"duration_ms", time.Since(startedAt).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader 记录实际响应状态码后继续写入底层 ResponseWriter。
func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// newRequestID 生成请求追踪 ID，随机源失败时使用时间戳作为降级值。
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}
