package tenant

import (
	"context"
	"strings"
)

type contextKey string

const tenantIDContextKey contextKey = "tenant_id"

// WithTenantID 将规范化后的租户 ID 写入上下文。
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDContextKey, strings.TrimSpace(tenantID))
}

// TenantIDFromContext 从上下文读取租户 ID，并返回其是否有效。
func TenantIDFromContext(ctx context.Context) (string, bool) {
	tenantID, _ := ctx.Value(tenantIDContextKey).(string)
	tenantID = strings.TrimSpace(tenantID)
	return tenantID, tenantID != ""
}
