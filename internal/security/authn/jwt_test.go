package authn

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestParseAndVerifyJWT 验证合法 HS256 JWT 能解析出身份声明。
func TestParseAndVerifyJWT(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	token := signTestJWT(t, "secret", map[string]any{
		"tenant_id": "tenant_001",
		"user_id":   "user_001",
		"role":      "admin",
		"exp":       now.Add(time.Hour).Unix(),
	})

	claims, err := ParseAndVerifyJWT(token, "secret", now)
	if err != nil {
		t.Fatalf("ParseAndVerifyJWT 返回错误: %v", err)
	}
	if claims.TenantID != "tenant_001" || claims.UserID != "user_001" || claims.Role != "admin" {
		t.Fatalf("claims 未正确解析: %+v", claims)
	}
}

// TestParseAndVerifyJWTRejectsExpiredToken 验证过期 JWT 会被拒绝。
func TestParseAndVerifyJWTRejectsExpiredToken(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	token := signTestJWT(t, "secret", map[string]any{
		"tenant_id": "tenant_001",
		"user_id":   "user_001",
		"exp":       now.Add(-time.Minute).Unix(),
	})

	if _, err := ParseAndVerifyJWT(token, "secret", now); err == nil {
		t.Fatal("期望过期 token 返回错误")
	}
}

// TestJWTMiddlewareWritesUserAndTenantContext 验证 JWT middleware 会写入用户和租户上下文。
func TestJWTMiddlewareWritesUserAndTenantContext(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	token := signTestJWT(t, "secret", map[string]any{
		"tenant_id": "tenant_001",
		"user_id":   "user_001",
		"exp":       now.Add(time.Hour).Unix(),
	})
	middleware := JWTMiddleware(JWTMiddlewareConfig{
		Secret: "secret",
		Now: func() time.Time {
			return now
		},
	})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok {
			t.Fatal("context 中没有用户")
		}
		if user.TenantID != "tenant_001" || user.UserID != "user_001" {
			t.Fatalf("用户上下文不正确: %+v", user)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

// signTestJWT 生成测试用 HS256 JWT。
func signTestJWT(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("序列化 header 失败: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("序列化 claims 失败: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + payload + "." + signature
}
