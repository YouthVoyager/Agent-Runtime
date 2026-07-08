package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"stableagent/internal/domain/runtimeplan"
)

// Provider 屏蔽不同模型供应商差异,生产接入真实 LLM 时实现该接口。
type Provider interface {
	Name() string
	Chat(ctx context.Context, input runtimeplan.ChatRequest, prompt PromptTemplate) (runtimeplan.ChatResponse, error)
}

// retryableError 标记可重试的 provider 错误(网络抖动、限速、上游 5xx)。
type retryableError struct {
	cause error
}

func (e *retryableError) Error() string { return e.cause.Error() }
func (e *retryableError) Unwrap() error { return e.cause }

// MarkRetryable 将 provider 错误标记为可重试。
func MarkRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &retryableError{cause: err}
}

// IsRetryable 判断 provider 错误是否可重试。
func IsRetryable(err error) bool {
	var target *retryableError
	return errors.As(err, &target)
}

// mockProvider 是确定性的本地 Mock 模型,按目标关键词生成多 step 计划。
type mockProvider struct{}

// NewMockProvider 创建本地闭环使用的确定性 Mock Provider。
func NewMockProvider() Provider {
	return &mockProvider{}
}

func (p *mockProvider) Name() string { return "mock-deterministic-agent" }

// planStep 描述确定性 Mock 计划中的一个工具调用步骤。
type planStep struct {
	ToolName  string
	Arguments map[string]any
}

// Chat 根据任务目标和当前步骤生成确定性的多 step Mock 模型响应。
func (p *mockProvider) Chat(ctx context.Context, input runtimeplan.ChatRequest, prompt PromptTemplate) (runtimeplan.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return runtimeplan.ChatResponse{}, MarkRetryable(err)
	}
	goal := strings.TrimSpace(input.Goal)

	// 循环目标永远返回相同 step 且不进入终态,用于验证 LoopDetector 兜底。
	if isLoopGoal(goal) {
		return p.stepResponse(goal, planStep{
			ToolName: "read_document",
			Arguments: map[string]any{
				"path": "设计方案.md",
				"goal": goal,
			},
		}, input.CurrentStep, 0, prompt)
	}

	steps := planSteps(goal)
	if int(input.CurrentStep) >= len(steps) {
		content := fmt.Sprintf("计划共 %d 步已全部完成,任务结束。", len(steps))
		return runtimeplan.ChatResponse{
			Model:         p.Name(),
			PromptVersion: prompt.Version,
			Content:       content,
			IsFinal:       true,
			Usage:         estimateUsage(goal, content),
		}, nil
	}
	return p.stepResponse(goal, steps[input.CurrentStep], input.CurrentStep, len(steps), prompt)
}

// stepResponse 生成一个工具调用 step 的响应。
func (p *mockProvider) stepResponse(goal string, step planStep, currentStep int32, totalSteps int, prompt PromptTemplate) (runtimeplan.ChatResponse, error) {
	payload, err := json.Marshal(step.Arguments)
	if err != nil {
		return runtimeplan.ChatResponse{}, err
	}
	// 循环计划(totalSteps=0)的 content 不带 step 序号,保持每步输出完全一致以便循环指纹命中。
	content := fmt.Sprintf("循环计划:调用工具 %s", step.ToolName)
	if totalSteps > 0 {
		content = fmt.Sprintf("共 %d 步计划:执行第 %d 步,调用工具 %s", totalSteps, currentStep+1, step.ToolName)
	}
	return runtimeplan.ChatResponse{
		Model:         p.Name(),
		PromptVersion: prompt.Version,
		Content:       content,
		ToolCall: &runtimeplan.ToolIntent{
			ToolName:  step.ToolName,
			Arguments: payload,
		},
		IsFinal: false,
		Usage:   estimateUsage(goal, content),
	}, nil
}

// planSteps 根据目标关键词生成确定性多 step 计划。
func planSteps(goal string) []planStep {
	lowerGoal := strings.ToLower(goal)
	switch {
	case strings.Contains(goal, "危险") || strings.Contains(lowerGoal, "critical"):
		return []planStep{{
			ToolName: "dangerous_admin_action",
			Arguments: map[string]any{
				"action": "rotate-production-secret",
				"reason": goal,
			},
		}}
	case strings.Contains(goal, "高风险") || strings.Contains(goal, "审批") || strings.Contains(goal, "邮件") || strings.Contains(lowerGoal, "send"):
		return []planStep{{
			ToolName: "send_external_message",
			Arguments: map[string]any{
				"recipient": "ops@example.local",
				"subject":   "StableAgent 高风险工具审批演示",
				"body":      goal,
			},
		}}
	default:
		// 默认计划体现多 step 工作流:先读取上下文,再写入产物。
		return []planStep{
			{
				ToolName: "read_document",
				Arguments: map[string]any{
					"path": "设计方案.md",
					"goal": goal,
				},
			},
			{
				ToolName: "write_artifact",
				Arguments: map[string]any{
					"name":    "stableagent-report",
					"summary": "根据任务目标生成本地生产闭环报告",
					"goal":    goal,
				},
			},
		}
	}
}

// isLoopGoal 判断目标是否用于触发循环检测演示。
func isLoopGoal(goal string) bool {
	return strings.Contains(goal, "循环") || strings.Contains(strings.ToLower(goal), "loop")
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
