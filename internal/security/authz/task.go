package authz

import (
	"strings"

	"agent-runtime/internal/security/authn"
)

func CanListTenantTasks(user authn.User) bool {
	switch strings.ToLower(strings.TrimSpace(user.Role)) {
	case "admin", "owner":
		return true
	default:
		return false
	}
}

func CanReadTask(user authn.User, taskUserID string) bool {
	if CanListTenantTasks(user) {
		return true
	}
	return strings.TrimSpace(user.UserID) == strings.TrimSpace(taskUserID)
}
