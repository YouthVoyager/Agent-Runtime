package runtimeplan

import "encoding/json"

type TokenUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type ToolIntent struct {
	ToolName  string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ChatRequest struct {
	TaskID      string          `json:"task_id"`
	Goal        string          `json:"goal"`
	CurrentStep int32           `json:"current_step"`
	Constraints json.RawMessage `json:"constraints"`
}

type ChatResponse struct {
	Model         string      `json:"model"`
	PromptVersion string      `json:"prompt_version"`
	Content       string      `json:"content"`
	ToolCall      *ToolIntent `json:"tool_call,omitempty"`
	IsFinal       bool        `json:"is_final"`
	Usage         TokenUsage  `json:"usage"`
}
