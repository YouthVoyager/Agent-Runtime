package task

import (
	"encoding/json"
	"strings"
	"testing"
)

// validBudget 返回一个处于合法范围内的预算样本。
func validBudget() Budget {
	return Budget{MaxSteps: 50, MaxTokens: 200000, MaxToolCalls: 40, MaxCostUSD: 10}
}

// TestValidateBudget 校验预算四个维度的边界规则。
func TestValidateBudget(t *testing.T) {
	if err := ValidateBudget(validBudget()); err != nil {
		t.Fatalf("合法预算不应报错: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Budget)
	}{
		{"max_steps 为 0", func(b *Budget) { b.MaxSteps = 0 }},
		{"max_steps 超上限", func(b *Budget) { b.MaxSteps = MaxStepsLimit + 1 }},
		{"max_tokens 为 0", func(b *Budget) { b.MaxTokens = 0 }},
		{"max_tokens 超上限", func(b *Budget) { b.MaxTokens = MaxTokensLimit + 1 }},
		{"max_tool_calls 为 0", func(b *Budget) { b.MaxToolCalls = 0 }},
		{"max_tool_calls 超上限", func(b *Budget) { b.MaxToolCalls = MaxToolCallsLimit + 1 }},
		{"max_cost_usd 为 0", func(b *Budget) { b.MaxCostUSD = 0 }},
		{"max_cost_usd 超上限", func(b *Budget) { b.MaxCostUSD = MaxCostUSDLimit + 1 }},
	}
	for _, tc := range cases {
		budget := validBudget()
		tc.mutate(&budget)
		if err := ValidateBudget(budget); err == nil {
			t.Fatalf("%s 必须被拒绝", tc.name)
		}
	}
}

// TestNormalizeGoal 校验目标文本的清理和长度限制。
func TestNormalizeGoal(t *testing.T) {
	goal, err := NormalizeGoal("  生成报告  ")
	if err != nil || goal != "生成报告" {
		t.Fatalf("目标应被裁剪: got %q err %v", goal, err)
	}
	if _, err := NormalizeGoal("   "); err == nil {
		t.Fatal("空目标必须被拒绝")
	}
	if _, err := NormalizeGoal(strings.Repeat("长", MaxGoalLength+1)); err == nil {
		t.Fatal("超长目标必须被拒绝")
	}
}

// TestNormalizeJSONObject 校验可选 JSON 对象的空值处理和类型限制。
func TestNormalizeJSONObject(t *testing.T) {
	normalized, err := NormalizeJSONObject(nil)
	if err != nil || string(normalized) != `{}` {
		t.Fatalf("空值应规范化为空对象: got %s err %v", normalized, err)
	}
	if _, err := NormalizeJSONObject(json.RawMessage(`"text"`)); err == nil {
		t.Fatal("非对象 JSON 必须被拒绝")
	}
	if _, err := NormalizeJSONObject(json.RawMessage(`{oops`)); err == nil {
		t.Fatal("非法 JSON 必须被拒绝")
	}
}
