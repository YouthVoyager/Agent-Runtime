package toolcall

import (
	"encoding/json"
	"testing"
)

// TestRegistryRiskLevels 校验内置工具目录的风险等级与设计方案一致。
func TestRegistryRiskLevels(t *testing.T) {
	expected := map[string]RiskLevel{
		"read_document":          RiskLow,
		"write_artifact":         RiskMedium,
		"send_external_message":  RiskHigh,
		"dangerous_admin_action": RiskCritical,
	}
	registry := Registry()
	if len(registry) != len(expected) {
		t.Fatalf("工具数量不符: got %d want %d", len(registry), len(expected))
	}
	for name, risk := range expected {
		definition, ok := registry[name]
		if !ok {
			t.Fatalf("缺少工具: %s", name)
		}
		if definition.RiskLevel != risk {
			t.Fatalf("工具 %s 风险等级不符: got %s want %s", name, definition.RiskLevel, risk)
		}
	}
}

// TestRiskRouting 校验设计方案的风险分级处理规则:LOW/MEDIUM 自动执行、HIGH 审批、CRITICAL 禁止。
func TestRiskRouting(t *testing.T) {
	cases := []struct {
		risk             RiskLevel
		requiresApproval bool
		critical         bool
	}{
		{RiskLow, false, false},
		{RiskMedium, false, false},
		{RiskHigh, true, false},
		{RiskCritical, false, true},
	}
	for _, tc := range cases {
		if got := RequiresApproval(tc.risk); got != tc.requiresApproval {
			t.Fatalf("RequiresApproval(%s) = %v, want %v", tc.risk, got, tc.requiresApproval)
		}
		if got := IsCritical(tc.risk); got != tc.critical {
			t.Fatalf("IsCritical(%s) = %v, want %v", tc.risk, got, tc.critical)
		}
	}
}

// TestGetDefinition 校验工具注册表查询和未注册工具拒绝。
func TestGetDefinition(t *testing.T) {
	if _, err := GetDefinition("  Read_Document  "); err != nil {
		t.Fatalf("规范化后的已注册工具不应报错: %v", err)
	}
	if _, err := GetDefinition("unknown_tool"); err == nil {
		t.Fatal("未注册工具必须返回错误")
	}
}

// TestNormalizeArguments 校验参数规范化:空值转空对象、拒绝非对象和非法 JSON。
func TestNormalizeArguments(t *testing.T) {
	normalized, err := NormalizeArguments(nil)
	if err != nil || string(normalized) != `{}` {
		t.Fatalf("空参数应规范化为空对象: got %s err %v", normalized, err)
	}
	if _, err := NormalizeArguments(json.RawMessage(`[1,2]`)); err == nil {
		t.Fatal("数组参数必须被拒绝")
	}
	if _, err := NormalizeArguments(json.RawMessage(`{bad json`)); err == nil {
		t.Fatal("非法 JSON 必须被拒绝")
	}
}

// TestHashArgumentsStable 校验相同参数产生稳定哈希、不同参数产生不同哈希。
func TestHashArgumentsStable(t *testing.T) {
	args := json.RawMessage(`{"path":"设计方案.md"}`)
	first := HashArguments(args)
	second := HashArguments(args)
	if first != second {
		t.Fatalf("相同参数哈希必须稳定: %s vs %s", first, second)
	}
	other := HashArguments(json.RawMessage(`{"path":"另一个文件.md"}`))
	if first == other {
		t.Fatal("不同参数不应产生相同哈希")
	}
}

// TestBuildIdempotencyKey 校验幂等键确定性以及工具名规范化参与计算。
func TestBuildIdempotencyKey(t *testing.T) {
	hash := HashArguments(json.RawMessage(`{"a":1}`))
	first := BuildIdempotencyKey("task_1", 0, "Read_Document", hash)
	second := BuildIdempotencyKey(" task_1 ", 0, "read_document", hash)
	if first != second {
		t.Fatalf("规范化后的幂等键必须一致: %s vs %s", first, second)
	}
	if first == BuildIdempotencyKey("task_1", 1, "read_document", hash) {
		t.Fatal("不同 step 必须产生不同幂等键")
	}
	if first == BuildIdempotencyKey("task_2", 0, "read_document", hash) {
		t.Fatal("不同 task 必须产生不同幂等键")
	}
}
