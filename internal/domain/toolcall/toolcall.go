package toolcall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
)

type Status string

const (
	StatusPending         Status = "PENDING"
	StatusRunning         Status = "RUNNING"
	StatusWaitingApproval Status = "WAITING_APPROVAL"
	StatusSucceeded       Status = "SUCCEEDED"
	StatusFailed          Status = "FAILED"
	StatusCanceled        Status = "CANCELED"
)

type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "PENDING"
	ApprovalApproved ApprovalStatus = "APPROVED"
	ApprovalRejected ApprovalStatus = "REJECTED"
	ApprovalExpired  ApprovalStatus = "EXPIRED"
	ApprovalCanceled ApprovalStatus = "CANCELED"
)

type Definition struct {
	Name        string         `json:"name"`
	RiskLevel   RiskLevel      `json:"risk_level"`
	Description string         `json:"description"`
	Schema      ArgumentSchema `json:"schema"`
}

// Registry 返回本地闭环内置工具目录,每个工具带参数 JSON Schema。
func Registry() map[string]Definition {
	return map[string]Definition{
		"read_document": {
			Name:        "read_document",
			RiskLevel:   RiskLow,
			Description: "读取本地任务上下文中的文档摘要，不产生外部副作用",
			Schema: ArgumentSchema{
				Type: "object",
				Properties: map[string]ParamSpec{
					"path": {Type: "string", Description: "文档路径", MaxLength: 512},
					"goal": {Type: "string", Description: "读取目的", MaxLength: 2048},
				},
				Required: []string{"path"},
			},
		},
		"write_artifact": {
			Name:        "write_artifact",
			RiskLevel:   RiskMedium,
			Description: "写入任务产物，小内容保存到 PostgreSQL artifact 表",
			Schema: ArgumentSchema{
				Type: "object",
				Properties: map[string]ParamSpec{
					"name":    {Type: "string", Description: "产物名称", MaxLength: 256},
					"summary": {Type: "string", Description: "产物摘要", MaxLength: 4096},
					"goal":    {Type: "string", Description: "关联任务目标", MaxLength: 2048},
				},
				Required: []string{"name"},
			},
		},
		"send_external_message": {
			Name:        "send_external_message",
			RiskLevel:   RiskHigh,
			Description: "模拟外部可见消息发送，必须人工审批后执行",
			Schema: ArgumentSchema{
				Type: "object",
				Properties: map[string]ParamSpec{
					"recipient": {Type: "string", Description: "接收人地址", MaxLength: 256},
					"subject":   {Type: "string", Description: "消息主题", MaxLength: 512},
					"body":      {Type: "string", Description: "消息正文", MaxLength: 8192},
				},
				Required: []string{"recipient", "subject"},
			},
		},
		"dangerous_admin_action": {
			Name:        "dangerous_admin_action",
			RiskLevel:   RiskCritical,
			Description: "模拟危险管理操作，本地闭环默认禁止执行",
			Schema: ArgumentSchema{
				Type: "object",
				Properties: map[string]ParamSpec{
					"action": {Type: "string", Description: "管理操作名称", MaxLength: 256},
					"reason": {Type: "string", Description: "操作原因", MaxLength: 2048},
				},
				Required: []string{"action"},
			},
		},
	}
}

// GetDefinition 查询工具定义并校验工具是否注册。
func GetDefinition(toolName string) (Definition, error) {
	name := NormalizeToolName(toolName)
	definition, ok := Registry()[name]
	if !ok {
		return Definition{}, fmt.Errorf("工具未注册: %s", name)
	}
	return definition, nil
}

// NormalizeToolName 规范化工具名。
func NormalizeToolName(toolName string) string {
	return strings.ToLower(strings.TrimSpace(toolName))
}

// NormalizeArguments 校验并规范化工具参数 JSON 对象。
func NormalizeArguments(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("arguments JSON 格式错误: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("arguments 必须是 JSON 对象")
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(normalized), nil
}

// HashArguments 生成稳定的参数哈希，供幂等键和审计使用。
func HashArguments(arguments json.RawMessage) string {
	sum := sha256.Sum256(arguments)
	return hex.EncodeToString(sum[:])
}

// BuildIdempotencyKey 生成工具调用幂等键。
func BuildIdempotencyKey(taskID string, step int32, toolName string, argumentsHash string) string {
	raw := fmt.Sprintf("%s:%d:%s:%s", strings.TrimSpace(taskID), step, NormalizeToolName(toolName), argumentsHash)
	sum := sha256.Sum256([]byte(raw))
	return "tool_" + hex.EncodeToString(sum[:])
}

// RequiresApproval 判断风险等级是否需要人工审批。
func RequiresApproval(risk RiskLevel) bool {
	return risk == RiskHigh
}

// IsCritical 判断风险等级是否为本地闭环默认禁止的 CRITICAL。
func IsCritical(risk RiskLevel) bool {
	return risk == RiskCritical
}
