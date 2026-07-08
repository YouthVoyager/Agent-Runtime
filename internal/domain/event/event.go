package event

import (
	"encoding/json"
	"strings"
	"time"
)

type Type string

const (
	TypeTaskCreated          Type = "TASK_CREATED"
	TypeTaskStarted          Type = "TASK_STARTED"
	TypeTaskCompleted        Type = "TASK_COMPLETED"
	TypeTaskFailed           Type = "TASK_FAILED"
	TypeTaskCancelled        Type = "TASK_CANCELLED"
	TypeTaskResumed          Type = "TASK_RESUMED"
	TypeStepStarted          Type = "STEP_STARTED"
	TypeStepCompleted        Type = "STEP_COMPLETED"
	TypeStepFailed           Type = "STEP_FAILED"
	TypeLLMCallStarted       Type = "LLM_CALL_STARTED"
	TypeLLMCallCompleted     Type = "LLM_CALL_COMPLETED"
	TypeToolCallStarted      Type = "TOOL_CALL_STARTED"
	TypeToolCallCompleted    Type = "TOOL_CALL_COMPLETED"
	TypeToolApprovalRequired Type = "TOOL_APPROVAL_REQUIRED"
	TypeToolApprovalDecided  Type = "TOOL_APPROVAL_DECIDED"
	TypeCheckpointCreated    Type = "CHECKPOINT_CREATED"
	TypeArtifactSaved        Type = "ARTIFACT_SAVED"
)

type Metadata struct {
	UserID     string         `json:"user_id,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	Source     string         `json:"source,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	Attempt    int            `json:"attempt,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
}

type AgentEvent struct {
	EventID   int64           `json:"event_id"`
	TaskID    string          `json:"task_id"`
	TenantID  string          `json:"-"`
	Type      string          `json:"type"`
	Input     json.RawMessage `json:"input"`
	Output    json.RawMessage `json:"output"`
	Metadata  json.RawMessage `json:"metadata"`
	TraceID   *string         `json:"trace_id,omitempty"`
	SpanID    *string         `json:"span_id,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// ParseType 标准化并校验 AgentEvent 类型。
func ParseType(raw string) (Type, bool) {
	eventType := Type(strings.ToUpper(strings.TrimSpace(raw)))
	switch eventType {
	case TypeTaskCreated,
		TypeTaskStarted,
		TypeTaskCompleted,
		TypeTaskFailed,
		TypeTaskCancelled,
		TypeTaskResumed,
		TypeStepStarted,
		TypeStepCompleted,
		TypeStepFailed,
		TypeLLMCallStarted,
		TypeLLMCallCompleted,
		TypeToolCallStarted,
		TypeToolCallCompleted,
		TypeToolApprovalRequired,
		TypeToolApprovalDecided,
		TypeCheckpointCreated,
		TypeArtifactSaved:
		return eventType, true
	default:
		return "", false
	}
}

// MustTypeForStorage 返回数据库使用的标准事件类型文本。
func MustTypeForStorage(eventType Type) string {
	return string(eventType)
}

// NormalizeType 标准化事件类型字符串，非法类型返回 false。
func NormalizeType(raw string) (string, bool) {
	eventType, ok := ParseType(raw)
	if !ok {
		return "", false
	}
	return string(eventType), true
}

// MarshalMetadata 序列化统一事件 metadata，空 metadata 使用空对象保持结构稳定。
func MarshalMetadata(metadata Metadata) (json.RawMessage, error) {
	if metadata.UserID == "" &&
		metadata.RequestID == "" &&
		metadata.Source == "" &&
		metadata.StepID == "" &&
		metadata.WorkflowID == "" &&
		metadata.Attempt == 0 &&
		len(metadata.Extra) == 0 {
		return json.RawMessage(`{}`), nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// NormalizeJSONValue 规范化事件 input/output，空值以 JSON null 存储。
func NormalizeJSONValue(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`null`)
	}
	return value
}

// NormalizeMetadata 规范化事件 metadata，空值以 JSON object 存储。
func NormalizeMetadata(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}
