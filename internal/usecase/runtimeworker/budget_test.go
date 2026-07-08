package runtimeworker

import (
	"encoding/json"
	"testing"

	"stableagent/internal/domain/runtimeplan"
	domaintask "stableagent/internal/domain/task"
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

// TestIncrementBudgetUsage 校验预算用量累计:tool call 只在实际调用时计数。
func TestIncrementBudgetUsage(t *testing.T) {
	raw := json.RawMessage(`{"used_steps":1,"used_tokens":10,"used_tool_calls":2,"used_cost_usd":0.5}`)
	usage := runtimeplan.TokenUsage{TotalTokens: 30, CostUSD: 0.1}

	withTool, err := incrementBudgetUsage(raw, usage, true)
	if err != nil {
		t.Fatalf("累计预算不应报错: %v", err)
	}
	var afterTool domaintask.BudgetUsage
	if err := json.Unmarshal(withTool, &afterTool); err != nil {
		t.Fatalf("解析累计结果失败: %v", err)
	}
	if afterTool.UsedSteps != 2 || afterTool.UsedTokens != 40 || afterTool.UsedToolCalls != 3 {
		t.Fatalf("发生工具调用的 step 累计错误: %+v", afterTool)
	}

	withoutTool, err := incrementBudgetUsage(raw, usage, false)
	if err != nil {
		t.Fatalf("累计预算不应报错: %v", err)
	}
	var afterNoTool domaintask.BudgetUsage
	if err := json.Unmarshal(withoutTool, &afterNoTool); err != nil {
		t.Fatalf("解析累计结果失败: %v", err)
	}
	if afterNoTool.UsedToolCalls != 2 {
		t.Fatalf("未发生工具调用时 tool call 不应累计: %+v", afterNoTool)
	}
	if afterNoTool.UsedSteps != 2 || afterNoTool.UsedTokens != 40 {
		t.Fatalf("step/token 用量必须照常累计: %+v", afterNoTool)
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
