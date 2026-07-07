package authn

import (
	"context"
	"strings"
)

type contextKey string

const userContextKey contextKey = "authn_user"

type User struct {
	TenantID string
	UserID   string
	Role     string
}

func WithUser(ctx context.Context, user User) context.Context {
	user.TenantID = strings.TrimSpace(user.TenantID)
	user.UserID = strings.TrimSpace(user.UserID)
	user.Role = strings.TrimSpace(user.Role)
	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey).(User)
	if !ok {
		return User{}, false
	}
	user.TenantID = strings.TrimSpace(user.TenantID)
	user.UserID = strings.TrimSpace(user.UserID)
	user.Role = strings.TrimSpace(user.Role)
	return user, user.TenantID != "" && user.UserID != ""
}
