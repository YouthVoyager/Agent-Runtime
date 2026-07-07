package authn

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	securitytenant "agent-runtime/internal/security/tenant"
	httpserver "agent-runtime/internal/transport/http"
	apperrors "agent-runtime/pkg/errors"
)

type JWTMiddlewareConfig struct {
	Secret string
	Now    func() time.Time
}

type JWTClaims struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	Subject   string `json:"sub"`
	Role      string `json:"role"`
	ExpiresAt *int64 `json:"exp"`
	NotBefore *int64 `json:"nbf"`
	IssuedAt  *int64 `json:"iat"`
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

var (
	ErrInvalidToken = errors.New("jwt token 无效")
	ErrExpiredToken = errors.New("jwt token 已过期")
)

func JWTMiddleware(config JWTMiddlewareConfig) func(http.Handler) http.Handler {
	now := config.Now
	if now == nil {
		now = time.Now
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				httpserver.WriteError(w, r, apperrors.New(apperrors.CodeUnauthorized, "缺少 Bearer token"))
				return
			}

			claims, err := ParseAndVerifyJWT(token, config.Secret, now())
			if err != nil {
				httpserver.WriteError(w, r, apperrors.New(apperrors.CodeUnauthorized, "无效或已过期的 Bearer token"))
				return
			}

			user := User{
				TenantID: strings.TrimSpace(claims.TenantID),
				UserID:   firstNonBlank(claims.UserID, claims.Subject),
				Role:     firstNonBlank(claims.Role, "member"),
			}
			ctx := WithUser(r.Context(), user)
			ctx = securitytenant.WithTenantID(ctx, user.TenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ParseAndVerifyJWT(token string, secret string, now time.Time) (JWTClaims, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return JWTClaims{}, ErrInvalidToken
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return JWTClaims{}, ErrInvalidToken
	}

	var header jwtHeader
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	if header.Algorithm != "HS256" {
		return JWTClaims{}, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return JWTClaims{}, ErrInvalidToken
	}

	var claims JWTClaims
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return JWTClaims{}, ErrInvalidToken
	}
	claims.TenantID = strings.TrimSpace(claims.TenantID)
	claims.UserID = strings.TrimSpace(claims.UserID)
	claims.Subject = strings.TrimSpace(claims.Subject)
	claims.Role = strings.TrimSpace(claims.Role)
	if claims.TenantID == "" || firstNonBlank(claims.UserID, claims.Subject) == "" {
		return JWTClaims{}, ErrInvalidToken
	}
	if claims.ExpiresAt != nil && now.Unix() >= *claims.ExpiresAt {
		return JWTClaims{}, ErrExpiredToken
	}
	if claims.NotBefore != nil && now.Unix() < *claims.NotBefore {
		return JWTClaims{}, ErrInvalidToken
	}

	return claims, nil
}

func decodeJWTPart(part string, dst any) error {
	payload, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dst)
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
