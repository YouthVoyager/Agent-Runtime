package authz

import (
	"strings"

	"stableagent/internal/security/authn"
)

// CanListTenantTasks 判断用户是否具备查看租户内全部任务的权限。
func CanListTenantTasks(user authn.User) bool {
	switch strings.ToLower(strings.TrimSpace(user.Role)) {
	case "admin", "owner":
		return true
	default:
		return false
	}
}

// CanReadTask 判断用户是否能读取指定任务，管理员和所有者可跨用户读取。
func CanReadTask(user authn.User, taskUserID string) bool {
	if CanListTenantTasks(user) {
		return true
	}
	return strings.TrimSpace(user.UserID) == strings.TrimSpace(taskUserID)
}
