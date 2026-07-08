package llmgateway

import (
	"context"
	"testing"

	"stableagent/internal/domain/runtimeplan"
)

// chat 执行一次 Mock Chat 并返回响应。
func chat(t *testing.T, goal string, step int32) runtimeplan.ChatResponse {
	t.Helper()
	resp, err := NewService().Chat(context.Background(), runtimeplan.ChatRequest{
		TaskID:      "task_test",
		Goal:        goal,
		CurrentStep: step,
	})
	if err != nil {
		t.Fatalf("Chat 不应报错: %v", err)
	}
	return resp
}

// TestChatKeywordRouting 校验确定性 Mock 的目标关键词到工具的路由规则。
func TestChatKeywordRouting(t *testing.T) {
	cases := []struct {
		goal string
		tool string
	}{
		{"生成本地生产闭环报告", "write_artifact"},
		{"危险 critical 管理操作", "dangerous_admin_action"},
		{"高风险审批发送邮件", "send_external_message"},
		{"读取设计方案文档", "read_document"},
	}
	for _, tc := range cases {
		resp := chat(t, tc.goal, 0)
		if resp.ToolCall == nil {
			t.Fatalf("goal %q 第 0 步必须返回 tool_call", tc.goal)
		}
		if resp.ToolCall.ToolName != tc.tool {
			t.Fatalf("goal %q 路由错误: got %s want %s", tc.goal, resp.ToolCall.ToolName, tc.tool)
		}
		if resp.IsFinal {
			t.Fatalf("goal %q 第 0 步不应是终态", tc.goal)
		}
		if resp.Usage.TotalTokens <= 0 || resp.Usage.CostUSD <= 0 {
			t.Fatalf("goal %q 必须返回 token 用量估算", tc.goal)
		}
	}
}

// TestChatFinalStep 校验 step 大于 0 时 Mock 返回终态且不再调用工具。
func TestChatFinalStep(t *testing.T) {
	resp := chat(t, "任意目标", 1)
	if !resp.IsFinal {
		t.Fatal("step > 0 时必须返回 IsFinal")
	}
	if resp.ToolCall != nil {
		t.Fatal("终态响应不应包含 tool_call")
	}
}
