package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
