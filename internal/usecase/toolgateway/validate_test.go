package toolgateway

import (
	"testing"
)

// TestValidateCallInput 校验工具调用请求的必填身份和幂等字段。
func TestValidateCallInput(t *testing.T) {
	valid := CallInput{
		TenantID:       "tenant_local",
		UserID:         "admin_local",
		TaskID:         "task_1",
		ToolName:       "read_document",
		IdempotencyKey: "tool_abc",
	}
	if err := validateCallInput(valid); err != nil {
		t.Fatalf("合法请求不应报错: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*CallInput)
	}{
		{"tenant_id 为空", func(in *CallInput) { in.TenantID = " " }},
		{"user_id 为空", func(in *CallInput) { in.UserID = "" }},
		{"task_id 为空", func(in *CallInput) { in.TaskID = "" }},
		{"tool_name 为空", func(in *CallInput) { in.ToolName = "" }},
		{"idempotency_key 为空", func(in *CallInput) { in.IdempotencyKey = "  " }},
	}
	for _, tc := range cases {
		input := valid
		tc.mutate(&input)
		if err := validateCallInput(input); err == nil {
			t.Fatalf("%s 必须被拒绝", tc.name)
		}
	}
}

// TestCatalog 校验工具目录返回全部注册工具。
func TestCatalog(t *testing.T) {
	service := &Service{}
	catalog := service.Catalog()
	if len(catalog) != 4 {
		t.Fatalf("工具目录数量不符: got %d want 4", len(catalog))
	}
	names := map[string]bool{}
	for _, definition := range catalog {
		names[definition.Name] = true
	}
	for _, expected := range []string{"read_document", "write_artifact", "send_external_message", "dangerous_admin_action"} {
		if !names[expected] {
			t.Fatalf("工具目录缺少 %s", expected)
		}
	}
}
