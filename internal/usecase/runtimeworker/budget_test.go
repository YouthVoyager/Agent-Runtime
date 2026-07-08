package runtimeworker

import (
	"encoding/json"
	"testing"

	"stableagent/internal/infra/postgres/db"
)

// budgetTask 构造带指定预算和用量的任务记录。
func budgetTask(t *testing.T, budget map[string]any, usage map[string]any) db.AgentTask {
	t.Helper()
	budgetJSON, err := json.Marshal(budget)
	if err != nil {
		t.Fatalf("序列化 budget 失败: %v", err)
	}
	usageJSON, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("序列化 budget_usage 失败: %v", err)
	}
	return db.AgentTask{TaskID: "task_budget", Budget: budgetJSON, BudgetUsage: usageJSON}
}

// TestCheckBudget 校验设计方案要求的 step/token/tool/cost 四维预算检测。
func TestCheckBudget(t *testing.T) {
	service := &Service{}
	budget := map[string]any{"max_steps": 2, "max_tokens": 100, "max_tool_calls": 3, "max_cost_usd": 1.0}

	ok := budgetTask(t, budget, map[string]any{"used_steps": 1, "used_tokens": 50, "used_tool_calls": 1, "used_cost_usd": 0.5})
	if err := service.checkBudget(ok); err != nil {
		t.Fatalf("预算内任务不应报错: %v", err)
	}

	cases := []struct {
		name  string
		usage map[string]any
	}{
		{"step 达到上限", map[string]any{"used_steps": 2}},
		{"token 达到上限", map[string]any{"used_tokens": 100}},
		{"tool call 达到上限", map[string]any{"used_tool_calls": 3}},
		{"成本达到上限", map[string]any{"used_cost_usd": 1.0}},
	}
	for _, tc := range cases {
		task := budgetTask(t, budget, tc.usage)
		if err := service.checkBudget(task); err == nil {
			t.Fatalf("%s 时必须拒绝继续执行", tc.name)
		}
	}
}

// TestCheckBudgetInvalidJSON 校验预算 JSON 损坏时返回错误而不是放行。
func TestCheckBudgetInvalidJSON(t *testing.T) {
	service := &Service{}
	task := db.AgentTask{Budget: json.RawMessage(`{bad`), BudgetUsage: json.RawMessage(`{}`)}
	if err := service.checkBudget(task); err == nil {
		t.Fatal("损坏的 budget JSON 必须返回错误")
	}
}
