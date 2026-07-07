package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"agent-runtime/internal/config"
)

func TestRegisterHealthRoutesWithChi(t *testing.T) {
	router := chi.NewRouter()
	RegisterHealthRoutes(router, config.Config{
		ServiceName: "api-service",
		Env:         "test",
	}, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var payload HealthPayload
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("解析 health 响应失败: %v", err)
	}
	if payload.Status != "ok" {
		t.Fatalf("status = %q, want ok", payload.Status)
	}
	if payload.Service != "api-service" {
		t.Fatalf("service = %q, want api-service", payload.Service)
	}
}
