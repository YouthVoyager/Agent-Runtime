package llmgateway

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"stableagent/internal/domain/runtimeplan"
)

type Service struct{}

// NewService 创建 LLM Gateway 用例服务。
func NewService() *Service {
	return &Service{}
}

// Chat 根据任务目标和当前步骤生成确定性的 Mock 模型响应。
func (s *Service) Chat(ctx context.Context, input runtimeplan.ChatRequest) (runtimeplan.ChatResponse, error) {
	_ = ctx
	goal := strings.TrimSpace(input.Goal)
	if input.CurrentStep > 0 {
		return runtimeplan.ChatResponse{
			Model:         "mock-deterministic-agent",
			PromptVersion: "local-v1",
			Content:       "任务已完成，无需继续调用工具。",
			IsFinal:       true,
			Usage:         estimateUsage(goal, "任务已完成"),
		}, nil
	}

	toolName := "write_artifact"
	arguments := map[string]any{
		"name":    "stableagent-report",
		"summary": "根据任务目标生成本地生产闭环报告",
		"goal":    goal,
	}
	lowerGoal := strings.ToLower(goal)
	switch {
	case strings.Contains(goal, "危险") || strings.Contains(lowerGoal, "critical"):
		toolName = "dangerous_admin_action"
		arguments = map[string]any{
			"action": "rotate-production-secret",
			"reason": goal,
		}
	case strings.Contains(goal, "高风险") || strings.Contains(goal, "审批") || strings.Contains(goal, "邮件") || strings.Contains(lowerGoal, "send"):
		toolName = "send_external_message"
		arguments = map[string]any{
			"recipient": "ops@example.local",
			"subject":   "StableAgent 高风险工具审批演示",
			"body":      goal,
		}
	case strings.Contains(goal, "读取") || strings.Contains(lowerGoal, "read"):
		toolName = "read_document"
		arguments = map[string]any{
			"path": "设计方案.md",
			"goal": goal,
		}
	}

	payload, err := json.Marshal(arguments)
	if err != nil {
		return runtimeplan.ChatResponse{}, err
	}
	content := "生成第 1 步工具调用：" + toolName
	return runtimeplan.ChatResponse{
		Model:         "mock-deterministic-agent",
		PromptVersion: "local-v1",
		Content:       content,
		ToolCall: &runtimeplan.ToolIntent{
			ToolName:  toolName,
			Arguments: payload,
		},
		IsFinal: false,
		Usage:   estimateUsage(goal, content),
	}, nil
}

// estimateUsage 基于输入输出长度生成稳定 token 估算。
func estimateUsage(input string, output string) runtimeplan.TokenUsage {
	inputTokens := utf8.RuneCountInString(input)/2 + 8
	outputTokens := utf8.RuneCountInString(output)/2 + 8
	total := inputTokens + outputTokens
	return runtimeplan.TokenUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  total,
		CostUSD:      float64(total) * 0.000001,
	}
}
