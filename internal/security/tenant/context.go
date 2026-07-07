package tenant

import (
	"context"
	"strings"
)

type contextKey string

const tenantIDContextKey contextKey = "tenant_id"

func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDContextKey, strings.TrimSpace(tenantID))
}

func TenantIDFromContext(ctx context.Context) (string, bool) {
	tenantID, _ := ctx.Value(tenantIDContextKey).(string)
	tenantID = strings.TrimSpace(tenantID)
	return tenantID, tenantID != ""
}
