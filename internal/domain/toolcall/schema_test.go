package toolcall

import (
	"encoding/json"
	"strings"
	"testing"
)

// mustSchema 返回注册工具的参数 schema。
func mustSchema(t *testing.T, toolName string) ArgumentSchema {
	t.Helper()
	definition, err := GetDefinition(toolName)
	if err != nil {
		t.Fatalf("获取工具定义失败: %v", err)
	}
	return definition.Schema
}

func TestValidateArgumentsAccept(t *testing.T) {
	schema := mustSchema(t, "send_external_message")
	args := json.RawMessage(`{"recipient":"ops@example.local","subject":"审批演示","body":"正文"}`)
	if err := ValidateArguments(schema, args); err != nil {
		t.Fatalf("合法参数不应报错: %v", err)
	}
}

func TestValidateArgumentsMissingRequired(t *testing.T) {
	schema := mustSchema(t, "send_external_message")
	err := ValidateArguments(schema, json.RawMessage(`{"recipient":"ops@example.local"}`))
	if err == nil || !strings.Contains(err.Error(), "subject") {
		t.Fatalf("缺少必填 subject 应报错: %v", err)
	}
}

func TestValidateArgumentsEmptyRequired(t *testing.T) {
	schema := mustSchema(t, "read_document")
	err := ValidateArguments(schema, json.RawMessage(`{"path":"  "}`))
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("必填字段为空白应报错: %v", err)
	}
}

func TestValidateArgumentsWrongType(t *testing.T) {
	schema := mustSchema(t, "read_document")
	err := ValidateArguments(schema, json.RawMessage(`{"path":123}`))
	if err == nil || !strings.Contains(err.Error(), "string") {
		t.Fatalf("类型不符应报错: %v", err)
	}
}

func TestValidateArgumentsUnknownField(t *testing.T) {
	schema := mustSchema(t, "read_document")
	err := ValidateArguments(schema, json.RawMessage(`{"path":"设计方案.md","shell":"rm -rf /"}`))
	if err == nil || !strings.Contains(err.Error(), "shell") {
		t.Fatalf("未定义字段应被拒绝: %v", err)
	}
}

func TestValidateArgumentsMaxLength(t *testing.T) {
	schema := mustSchema(t, "read_document")
	long := strings.Repeat("a", 600)
	err := ValidateArguments(schema, json.RawMessage(`{"path":"`+long+`"}`))
	if err == nil || !strings.Contains(err.Error(), "最大长度") {
		t.Fatalf("超长字段应报错: %v", err)
	}
}

func TestRegistrySchemasCoverMockPlans(t *testing.T) {
	// 保证 Mock LLM 计划生成的参数组合都能通过 schema 校验。
	cases := map[string]string{
		"read_document":          `{"path":"设计方案.md","goal":"读取"}`,
		"write_artifact":         `{"name":"stableagent-report","summary":"摘要","goal":"目标"}`,
		"send_external_message":  `{"recipient":"ops@example.local","subject":"主题","body":"正文"}`,
		"dangerous_admin_action": `{"action":"rotate-production-secret","reason":"演示"}`,
	}
	for tool, args := range cases {
		if err := ValidateArguments(mustSchema(t, tool), json.RawMessage(args)); err != nil {
			t.Fatalf("工具 %s 的 Mock 参数应通过校验: %v", tool, err)
		}
	}
}
