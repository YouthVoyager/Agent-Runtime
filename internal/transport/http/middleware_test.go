package httpserver

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWithRequestIDUsesIncomingHeader 验证已有 request_id 会透传到上下文和响应头。
func TestWithRequestIDUsesIncomingHeader(t *testing.T) {
	var got string
	handler := WithRequestID("X-Request-ID")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "req-123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got != "req-123" {
		t.Fatalf("request_id = %q, want req-123", got)
	}
	if rr.Header().Get("X-Request-ID") != "req-123" {
		t.Fatalf("响应头 request_id = %q", rr.Header().Get("X-Request-ID"))
	}
}

// TestWithRequestIDGeneratesMissingHeader 验证缺失 request_id 时会自动生成。
func TestWithRequestIDGeneratesMissingHeader(t *testing.T) {
	handler := WithRequestID("X-Request-ID")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Fatal("context 中没有 request_id")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("响应头没有生成 request_id")
	}
}

// sseCapableRecorder 模拟支持写 deadline 的底层 ResponseWriter，用于验证包装穿透。
type sseCapableRecorder struct {
	*httptest.ResponseRecorder
	writeDeadlineSet bool
}

func (r *sseCapableRecorder) SetWriteDeadline(time.Time) error {
	r.writeDeadlineSet = true
	return nil
}

// TestWithAccessLogPreservesFlusher 验证 access log 包装后的 writer 仍支持 SSE 所需的 Flusher 与 ResponseController 穿透。
func TestWithAccessLogPreservesFlusher(t *testing.T) {
	handler := WithAccessLog(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("包装后的 writer 不支持 http.Flusher")
		}
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
			t.Fatalf("ResponseController 无法穿透包装: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}))

	base := &sseCapableRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(base, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t/events/stream", nil))

	if !base.Flushed {
		t.Fatal("Flush 没有传递到底层 writer")
	}
	if !base.writeDeadlineSet {
		t.Fatal("SetWriteDeadline 没有穿透到底层 writer")
	}
}
