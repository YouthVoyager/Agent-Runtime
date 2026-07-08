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

// TestChatKeywordRouting 校验确定性 Mock 的目标关键词到第 0 步工具的路由规则。
func TestChatKeywordRouting(t *testing.T) {
	cases := []struct {
		goal string
		tool string
	}{
		{"生成本地生产闭环报告", "read_document"},
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

// TestChatMultiStepPlan 校验默认目标按多 step 计划推进并在计划末尾进入终态。
func TestChatMultiStepPlan(t *testing.T) {
	goal := "生成本地生产闭环报告"
	first := chat(t, goal, 0)
	if first.ToolCall == nil || first.ToolCall.ToolName != "read_document" {
		t.Fatalf("第 0 步应调用 read_document: %+v", first.ToolCall)
	}
	second := chat(t, goal, 1)
	if second.ToolCall == nil || second.ToolCall.ToolName != "write_artifact" {
		t.Fatalf("第 1 步应调用 write_artifact: %+v", second.ToolCall)
	}
	final := chat(t, goal, 2)
	if !final.IsFinal {
		t.Fatal("计划完成后必须返回 IsFinal")
	}
	if final.ToolCall != nil {
		t.Fatal("终态响应不应包含 tool_call")
	}
}

// TestChatHighRiskPlanFinal 校验高风险单 step 计划在第 1 步进入终态。
func TestChatHighRiskPlanFinal(t *testing.T) {
	resp := chat(t, "高风险审批发送邮件", 1)
	if !resp.IsFinal {
		t.Fatal("高风险计划完成后必须返回 IsFinal")
	}
}

// TestChatLoopGoalNeverFinal 校验循环目标永远返回相同 step,用于触发 LoopDetector。
func TestChatLoopGoalNeverFinal(t *testing.T) {
	goal := "循环读取文档,验证循环检测"
	var lastTool string
	for step := int32(0); step < 5; step++ {
		resp := chat(t, goal, step)
		if resp.IsFinal {
			t.Fatalf("循环目标第 %d 步不应进入终态", step)
		}
		if resp.ToolCall == nil {
			t.Fatalf("循环目标第 %d 步必须返回 tool_call", step)
		}
		if lastTool != "" && resp.ToolCall.ToolName != lastTool {
			t.Fatalf("循环目标每步工具应一致: %s vs %s", lastTool, resp.ToolCall.ToolName)
		}
		lastTool = resp.ToolCall.ToolName
	}
}
