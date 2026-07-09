// Package authz 下的角色规则:对齐 docs/admin-console-api-contract.md 第 0 节。
// 角色取值统一小写:user、approver、auditor、tenant_admin、platform_admin。
// 历史遗留 member/admin/owner 继续兼容,分别归一化为 user/tenant_admin。
package authz

import (
	"strings"

	"stableagent/internal/security/authn"
)

const (
	RoleUser           = "user"
	RoleApprover       = "approver"
	RoleAuditor        = "auditor"
	RoleTenantAdmin    = "tenant_admin"
	RolePlatformAdmin  = "platform_admin"
	RoleSystemWorker   = "system_worker"
)

// NormalizeRole 将历史遗留角色名归一化为契约定义的角色取值。
func NormalizeRole(role string) string {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch normalized {
	case "", "member":
		return RoleUser
	case "admin", "owner":
		// 历史遗留 admin/owner 语义上等价于租户管理员,拥有租户内管理权限。
		return RoleTenantAdmin
	default:
		return normalized
	}
}

// IsPlatformAdmin 判断用户是否为平台管理员。
func IsPlatformAdmin(user authn.User) bool {
	return NormalizeRole(user.Role) == RolePlatformAdmin
}

// IsTenantAdmin 判断用户是否为租户管理员(不含平台管理员)。
func IsTenantAdmin(user authn.User) bool {
	return NormalizeRole(user.Role) == RoleTenantAdmin
}

// IsApprover 判断用户是否为审批人角色。
func IsApprover(user authn.User) bool {
	return NormalizeRole(user.Role) == RoleApprover
}

// IsAuditor 判断用户是否为审计员角色。
func IsAuditor(user authn.User) bool {
	return NormalizeRole(user.Role) == RoleAuditor
}

// IsAdmin 判断用户是否具备租户管理员及以上权限(tenant_admin 或 platform_admin)。
func IsAdmin(user authn.User) bool {
	role := NormalizeRole(user.Role)
	return role == RoleTenantAdmin || role == RolePlatformAdmin
}

// HasAnyRole 判断用户角色(归一化后)是否属于给定角色集合。
func HasAnyRole(user authn.User, roles ...string) bool {
	role := NormalizeRole(user.Role)
	for _, candidate := range roles {
		if role == strings.ToLower(strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

// CanListTenantTasks 判断用户是否具备查看租户内全部任务的权限。
// 兼容历史 admin/owner 语义,新增 tenant_admin/platform_admin/auditor 三个管理台角色。
func CanListTenantTasks(user authn.User) bool {
	role := NormalizeRole(user.Role)
	switch role {
	case RoleTenantAdmin, RolePlatformAdmin, RoleAuditor:
		return true
	default:
		return false
	}
}

// CanReadTask 判断用户是否能读取指定任务,管理员和所有者可跨用户读取。
func CanReadTask(user authn.User, taskUserID string) bool {
	if CanListTenantTasks(user) {
		return true
	}
	return strings.TrimSpace(user.UserID) == strings.TrimSpace(taskUserID)
}

// CanWrite 判断用户是否具备写操作权限;auditor 角色全局只读,一律拒绝写操作。
func CanWrite(user authn.User) bool {
	return !IsAuditor(user)
}

// EffectiveTenantID 计算查询实际生效的租户过滤条件:
// platform_admin 允许通过 requestedTenantID 跨租户过滤(为空表示查看全部);
// 其余角色一律强制收敛到 token 自带的 tenant_id,忽略请求中传入的 tenant_id。
func EffectiveTenantID(user authn.User, requestedTenantID string) (tenantID string, crossTenant bool) {
	if IsPlatformAdmin(user) {
		return strings.TrimSpace(requestedTenantID), true
	}
	return user.TenantID, false
}

// ScopeToSelf 判断任务/事件/产物类查询是否需要强制收敛到 created_by = sub。
// 契约:user 角色的任务/事件/产物类查询强制 created_by = sub。
func ScopeToSelf(user authn.User) bool {
	return NormalizeRole(user.Role) == RoleUser
}
