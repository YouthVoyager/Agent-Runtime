package authz

import (
	"testing"

	"stableagent/internal/security/authn"
)

// TestCanListTenantTasks 校验只有 admin/owner 可以查看租户全量任务。
func TestCanListTenantTasks(t *testing.T) {
	cases := []struct {
		role    string
		allowed bool
	}{
		{"admin", true},
		{"OWNER", true},
		{" Admin ", true},
		{"member", false},
		{"", false},
	}
	for _, tc := range cases {
		user := authn.User{TenantID: "tenant_local", UserID: "user_1", Role: tc.role}
		if got := CanListTenantTasks(user); got != tc.allowed {
			t.Fatalf("role %q: got %v want %v", tc.role, got, tc.allowed)
		}
	}
}

// TestCanReadTask 校验普通用户仅能读取自己的任务,管理员可跨用户读取。
func TestCanReadTask(t *testing.T) {
	member := authn.User{TenantID: "tenant_local", UserID: "user_1", Role: "member"}
	if !CanReadTask(member, "user_1") {
		t.Fatal("用户必须能读取自己的任务")
	}
	if CanReadTask(member, "user_2") {
		t.Fatal("普通用户不能读取他人任务")
	}
	admin := authn.User{TenantID: "tenant_local", UserID: "admin_1", Role: "admin"}
	if !CanReadTask(admin, "user_2") {
		t.Fatal("管理员必须能读取租户内他人任务")
	}
}
