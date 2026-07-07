package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	MaxGoalLength     = 8000
	MaxStepsLimit     = 1000
	MaxTokensLimit    = 10000000
	MaxToolCallsLimit = 10000
	MaxCostUSDLimit   = 100000
)

type Budget struct {
	MaxSteps     int     `json:"max_steps"`
	MaxTokens    int     `json:"max_tokens"`
	MaxToolCalls int     `json:"max_tool_calls"`
	MaxCostUSD   float64 `json:"max_cost_usd"`
}

type BudgetUsage struct {
	UsedSteps     int     `json:"used_steps"`
	UsedTokens    int     `json:"used_tokens"`
	UsedToolCalls int     `json:"used_tool_calls"`
	UsedCostUSD   float64 `json:"used_cost_usd"`
}

func NormalizeGoal(goal string) (string, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "", errors.New("goal 不能为空")
	}
	if len([]rune(goal)) > MaxGoalLength {
		return "", fmt.Errorf("goal 长度不能超过 %d 个字符", MaxGoalLength)
	}
	return goal, nil
}

func ValidateBudget(budget Budget) error {
	if budget.MaxSteps <= 0 || budget.MaxSteps > MaxStepsLimit {
		return fmt.Errorf("budget.max_steps 必须在 1 到 %d 之间", MaxStepsLimit)
	}
	if budget.MaxTokens <= 0 || budget.MaxTokens > MaxTokensLimit {
		return fmt.Errorf("budget.max_tokens 必须在 1 到 %d 之间", MaxTokensLimit)
	}
	if budget.MaxToolCalls <= 0 || budget.MaxToolCalls > MaxToolCallsLimit {
		return fmt.Errorf("budget.max_tool_calls 必须在 1 到 %d 之间", MaxToolCallsLimit)
	}
	if budget.MaxCostUSD <= 0 || budget.MaxCostUSD > MaxCostUSDLimit {
		return fmt.Errorf("budget.max_cost_usd 必须大于 0 且不超过 %d", MaxCostUSDLimit)
	}
	return nil
}

func NormalizeJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("JSON 对象格式错误: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("必须是 JSON 对象")
	}
	return raw, nil
}
