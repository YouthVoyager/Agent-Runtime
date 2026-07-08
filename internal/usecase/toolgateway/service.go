package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	domainevent "stableagent/internal/domain/event"
	domaintool "stableagent/internal/domain/toolcall"
	"stableagent/internal/infra/postgres/db"
	"stableagent/internal/usecase/eventapi"
	apperrors "stableagent/pkg/errors"
	"stableagent/pkg/ids"
)

type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	events  *eventapi.Service
}

type CallInput struct {
	TenantID       string          `json:"tenant_id"`
	UserID         string          `json:"user_id"`
	TaskID         string          `json:"task_id"`
	ToolName       string          `json:"tool_name"`
	Arguments      json.RawMessage `json:"arguments"`
	IdempotencyKey string          `json:"idempotency_key"`
	TraceID        *string         `json:"trace_id,omitempty"`
}

type CallResult struct {
	CallID         string          `json:"call_id"`
	TaskID         string          `json:"task_id"`
	ToolName       string          `json:"tool_name"`
	Status         string          `json:"status"`
	RiskLevel      string          `json:"risk_level"`
	ApprovalStatus string          `json:"approval_status"`
	Result         json.RawMessage `json:"result,omitempty"`
	ErrorCode      *string         `json:"error_code,omitempty"`
	ErrorMessage   *string         `json:"error_message,omitempty"`
}

// NewService 创建工具网关用例服务。
func NewService(pool *pgxpool.Pool, events *eventapi.Service) *Service {
	return &Service{pool: pool, queries: db.New(pool), events: events}
}

// Catalog 返回本地闭环内置工具目录。
func (s *Service) Catalog() []domaintool.Definition {
	registry := domaintool.Registry()
	items := make([]domaintool.Definition, 0, len(registry))
	for _, definition := range registry {
		items = append(items, definition)
	}
	return items
}

// Call 执行工具调用入口，统一处理注册校验、风险判断、审批和幂等。
func (s *Service) Call(ctx context.Context, input CallInput) (CallResult, error) {
	if err := validateCallInput(input); err != nil {
		return CallResult{}, err
	}
	toolName := domaintool.NormalizeToolName(input.ToolName)
	definition, err := domaintool.GetDefinition(toolName)
	if err != nil {
		return CallResult{}, apperrors.Wrap(apperrors.CodeInvalidArg, err.Error(), err)
	}
	arguments, err := domaintool.NormalizeArguments(input.Arguments)
	if err != nil {
		return CallResult{}, apperrors.Wrap(apperrors.CodeInvalidArg, err.Error(), err)
	}
	if err := s.applyTenantPolicy(ctx, input.TenantID, definition); err != nil {
		return CallResult{}, err
	}

	existing, err := s.queries.GetToolCallByIdempotencyKey(ctx, db.GetToolCallByIdempotencyKeyParams{
		TenantID:       input.TenantID,
		IdempotencyKey: input.IdempotencyKey,
	})
	if err == nil {
		return s.handleExistingCall(ctx, input, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CallResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "查询工具调用幂等记录失败", err)
	}

	if domaintool.IsCritical(definition.RiskLevel) {
		return s.createRejectedCriticalCall(ctx, input, definition, arguments)
	}
	if domaintool.RequiresApproval(definition.RiskLevel) {
		return s.createApprovalCall(ctx, input, definition, arguments)
	}
	return s.createAndExecuteCall(ctx, input, definition, arguments)
}

// validateCallInput 校验工具调用请求中的身份和幂等字段。
func validateCallInput(input CallInput) error {
	if strings.TrimSpace(input.TenantID) == "" {
		return apperrors.New(apperrors.CodeInvalidArg, "tenant_id 不能为空")
	}
	if strings.TrimSpace(input.UserID) == "" {
		return apperrors.New(apperrors.CodeInvalidArg, "user_id 不能为空")
	}
	if strings.TrimSpace(input.TaskID) == "" {
		return apperrors.New(apperrors.CodeInvalidArg, "task_id 不能为空")
	}
	if strings.TrimSpace(input.ToolName) == "" {
		return apperrors.New(apperrors.CodeInvalidArg, "tool_name 不能为空")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return apperrors.New(apperrors.CodeInvalidArg, "idempotency_key 不能为空")
	}
	return nil
}

// applyTenantPolicy 应用租户工具策略，未配置时使用风险等级默认规则。
func (s *Service) applyTenantPolicy(ctx context.Context, tenantID string, definition domaintool.Definition) error {
	policy, err := s.queries.GetTenantToolPolicy(ctx, db.GetTenantToolPolicyParams{
		TenantID: tenantID,
		ToolName: definition.Name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return apperrors.Wrap(apperrors.CodeUnavailable, "查询租户工具策略失败", err)
	}
	if policy.Effect == db.ToolPolicyEffectDENY {
		return apperrors.New(apperrors.CodeForbidden, "租户策略禁止调用该工具")
	}
	return nil
}

// handleExistingCall 根据已有幂等记录返回或继续执行已审批工具。
func (s *Service) handleExistingCall(ctx context.Context, input CallInput, call db.ToolCall) (CallResult, error) {
	if call.Status == db.ToolCallStatusWAITINGAPPROVAL || call.ApprovalStatus == db.ApprovalStatusPENDING {
		return resultFromToolCall(call), nil
	}
	if call.Status == db.ToolCallStatusPENDING && call.ApprovalStatus == db.ApprovalStatusAPPROVED {
		return s.executeExistingCall(ctx, input, call)
	}
	return resultFromToolCall(call), nil
}

// createRejectedCriticalCall 记录被默认拒绝的 CRITICAL 工具调用。
func (s *Service) createRejectedCriticalCall(ctx context.Context, input CallInput, definition domaintool.Definition, arguments json.RawMessage) (CallResult, error) {
	call, err := s.insertToolCall(ctx, input, definition, arguments, db.ToolCallStatusFAILED, db.ApprovalStatusREJECTED)
	if err != nil {
		return CallResult{}, err
	}
	errorCode := "CRITICAL_TOOL_BLOCKED"
	errorMessage := "CRITICAL 风险工具在本地闭环中默认禁止执行"
	failed, err := s.queries.FailToolCall(ctx, db.FailToolCallParams{
		TenantID:     input.TenantID,
		CallID:       call.CallID,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	})
	if err != nil {
		return CallResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "记录 CRITICAL 工具拒绝结果失败", err)
	}
	_ = s.appendEvent(ctx, input, domainevent.TypeToolCallCompleted, map[string]any{"tool_name": definition.Name}, map[string]any{"error_code": errorCode, "error_message": errorMessage})
	return resultFromToolCall(failed), nil
}

// createApprovalCall 创建等待人工审批的高风险工具调用。
func (s *Service) createApprovalCall(ctx context.Context, input CallInput, definition domaintool.Definition, arguments json.RawMessage) (CallResult, error) {
	call, err := s.insertToolCall(ctx, input, definition, arguments, db.ToolCallStatusWAITINGAPPROVAL, db.ApprovalStatusPENDING)
	if err != nil {
		return CallResult{}, err
	}
	approvalID, err := ids.New("approval")
	if err != nil {
		return CallResult{}, apperrors.Wrap(apperrors.CodeInternal, "生成 approval_id 失败", err)
	}
	reason := "工具风险等级为 HIGH，需要人工审批"
	if _, err := s.queries.CreateApproval(ctx, db.CreateApprovalParams{
		ApprovalID:     approvalID,
		TenantID:       input.TenantID,
		TaskID:         input.TaskID,
		CallID:         call.CallID,
		ApproverID:     nil,
		Status:         db.ApprovalStatusPENDING,
		RiskLevel:      db.RiskLevel(definition.RiskLevel),
		ApprovalReason: &reason,
		Comment:        nil,
		ExpiresAt:      pgtype.Timestamptz{},
	}); err != nil {
		return CallResult{}, mapWriteError("创建审批记录失败", err)
	}
	if _, err := s.queries.UpdateAgentTaskStatus(ctx, db.UpdateAgentTaskStatusParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
		Status:   db.TaskStatusWAITINGAPPROVAL,
	}); err != nil {
		return CallResult{}, mapWriteError("更新任务审批等待状态失败", err)
	}
	_ = s.appendEvent(ctx, input, domainevent.TypeToolApprovalRequired, map[string]any{"tool_name": definition.Name, "call_id": call.CallID}, map[string]any{"approval_id": approvalID, "risk_level": definition.RiskLevel})
	return resultFromToolCall(call), nil
}

// createAndExecuteCall 创建低中风险工具调用并立即执行。
func (s *Service) createAndExecuteCall(ctx context.Context, input CallInput, definition domaintool.Definition, arguments json.RawMessage) (CallResult, error) {
	call, err := s.insertToolCall(ctx, input, definition, arguments, db.ToolCallStatusPENDING, db.ApprovalStatusAPPROVED)
	if err != nil {
		return CallResult{}, err
	}
	return s.executeExistingCall(ctx, input, call)
}

// insertToolCall 写入工具调用审计记录，并处理幂等冲突。
func (s *Service) insertToolCall(ctx context.Context, input CallInput, definition domaintool.Definition, arguments json.RawMessage, status db.ToolCallStatus, approvalStatus db.ApprovalStatus) (db.ToolCall, error) {
	callID, err := ids.New("call")
	if err != nil {
		return db.ToolCall{}, apperrors.Wrap(apperrors.CodeInternal, "生成 call_id 失败", err)
	}
	call, err := s.queries.CreateToolCall(ctx, db.CreateToolCallParams{
		CallID:         callID,
		TenantID:       input.TenantID,
		TaskID:         input.TaskID,
		UserID:         input.UserID,
		ToolName:       definition.Name,
		Arguments:      arguments,
		ArgumentsHash:  domaintool.HashArguments(arguments),
		IdempotencyKey: input.IdempotencyKey,
		Status:         status,
		RiskLevel:      db.RiskLevel(definition.RiskLevel),
		ApprovalStatus: approvalStatus,
	})
	if err == nil {
		return call, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		existing, readErr := s.queries.GetToolCallByIdempotencyKey(ctx, db.GetToolCallByIdempotencyKeyParams{
			TenantID:       input.TenantID,
			IdempotencyKey: input.IdempotencyKey,
		})
		if readErr != nil {
			return db.ToolCall{}, mapWriteError("读取幂等工具调用失败", readErr)
		}
		return existing, nil
	}
	return db.ToolCall{}, mapWriteError("创建工具调用记录失败", err)
}

// executeExistingCall 执行已有工具调用并写入结果。
func (s *Service) executeExistingCall(ctx context.Context, input CallInput, call db.ToolCall) (CallResult, error) {
	started, err := s.queries.StartToolCall(ctx, db.StartToolCallParams{
		TenantID: input.TenantID,
		CallID:   call.CallID,
	})
	if err != nil {
		return CallResult{}, mapWriteError("标记工具调用开始失败", err)
	}
	_ = s.appendEvent(ctx, input, domainevent.TypeToolCallStarted, map[string]any{"tool_name": started.ToolName, "call_id": started.CallID}, nil)

	result, artifactID, err := s.executeTool(ctx, input, started)
	if err != nil {
		message := err.Error()
		failed, failErr := s.queries.FailToolCall(ctx, db.FailToolCallParams{
			TenantID:     input.TenantID,
			CallID:       started.CallID,
			ErrorCode:    stringPtr("TOOL_EXECUTION_FAILED"),
			ErrorMessage: &message,
		})
		if failErr != nil {
			return CallResult{}, mapWriteError("记录工具失败结果失败", failErr)
		}
		_ = s.appendEvent(ctx, input, domainevent.TypeToolCallCompleted, map[string]any{"tool_name": failed.ToolName, "call_id": failed.CallID}, map[string]any{"error": message})
		return resultFromToolCall(failed), nil
	}
	completed, err := s.queries.CompleteToolCall(ctx, db.CompleteToolCallParams{
		TenantID:         input.TenantID,
		CallID:           started.CallID,
		Result:           result,
		ResultArtifactID: artifactID,
	})
	if err != nil {
		return CallResult{}, mapWriteError("记录工具成功结果失败", err)
	}
	_ = s.appendEvent(ctx, input, domainevent.TypeToolCallCompleted, map[string]any{"tool_name": completed.ToolName, "call_id": completed.CallID}, result)
	return resultFromToolCall(completed), nil
}

// executeTool 执行本地 Mock 工具并返回结果和可选 artifact。
func (s *Service) executeTool(ctx context.Context, input CallInput, call db.ToolCall) (json.RawMessage, *string, error) {
	var args map[string]any
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return nil, nil, err
	}
	switch call.ToolName {
	case "read_document":
		result, err := json.Marshal(map[string]any{
			"path":    firstString(args, "path", "设计方案.md"),
			"content": "StableAgent 本地闭环读取了设计方案摘要，用于后续稳定执行。",
		})
		return result, nil, err
	case "write_artifact":
		result, artifactID, err := s.writeArtifact(ctx, input, args)
		return result, artifactID, err
	case "send_external_message":
		result, err := json.Marshal(map[string]any{
			"message_id": "mock_msg_" + call.CallID,
			"recipient":  firstString(args, "recipient", "ops@example.local"),
			"status":     "sent",
		})
		return result, nil, err
	default:
		return nil, nil, fmt.Errorf("未实现工具执行器: %s", call.ToolName)
	}
}

// writeArtifact 将 Mock 产物写入 artifacts 表。
func (s *Service) writeArtifact(ctx context.Context, input CallInput, args map[string]any) (json.RawMessage, *string, error) {
	artifactID, err := ids.New("artifact")
	if err != nil {
		return nil, nil, err
	}
	content, err := json.Marshal(map[string]any{
		"title":   firstString(args, "name", "stableagent-report"),
		"summary": firstString(args, "summary", "StableAgent 本地生产闭环执行产物"),
		"goal":    firstString(args, "goal", ""),
	})
	if err != nil {
		return nil, nil, err
	}
	artifact, err := s.queries.CreateArtifact(ctx, db.CreateArtifactParams{
		ArtifactID:     artifactID,
		TenantID:       input.TenantID,
		TaskID:         input.TaskID,
		UserID:         input.UserID,
		Type:           db.ArtifactTypeJSON,
		Name:           firstString(args, "name", "stableagent-report"),
		MediaType:      stringPtr("application/json"),
		StorageBackend: "POSTGRES",
		StorageKey:     nil,
		ContentJson:    content,
		SizeBytes:      int64(len(content)),
		ChecksumSha256: nil,
		Metadata:       json.RawMessage(`{"provider":"mock-tool-gateway"}`),
		Status:         db.ArtifactStatusAVAILABLE,
	})
	if err != nil {
		return nil, nil, mapWriteError("写入 artifact 失败", err)
	}
	_ = s.appendEvent(ctx, input, domainevent.TypeArtifactSaved, map[string]any{"artifact_id": artifact.ArtifactID}, map[string]any{"name": artifact.Name, "type": artifact.Type})
	result, err := json.Marshal(map[string]any{
		"artifact_id": artifact.ArtifactID,
		"name":        artifact.Name,
		"type":        artifact.Type,
	})
	if err != nil {
		return nil, nil, err
	}
	return result, &artifact.ArtifactID, nil
}

// appendEvent 写入工具网关产生的 AgentEvent。
func (s *Service) appendEvent(ctx context.Context, input CallInput, eventType domainevent.Type, eventInput any, eventOutput any) error {
	if s.events == nil {
		return nil
	}
	encodedInput, err := encodeEventValue(eventInput)
	if err != nil {
		return err
	}
	encodedOutput, err := encodeEventValue(eventOutput)
	if err != nil {
		return err
	}
	metadata, err := domainevent.MarshalMetadata(domainevent.Metadata{
		UserID: input.UserID,
		Source: "tool-gateway",
	})
	if err != nil {
		return err
	}
	_, err = s.events.Append(ctx, eventapi.AppendEventInput{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
		Type:     string(eventType),
		Input:    encodedInput,
		Output:   encodedOutput,
		Metadata: metadata,
		TraceID:  input.TraceID,
	})
	return err
}

// resultFromToolCall 将数据库工具调用记录转换为 API 响应。
func resultFromToolCall(call db.ToolCall) CallResult {
	return CallResult{
		CallID:         call.CallID,
		TaskID:         call.TaskID,
		ToolName:       call.ToolName,
		Status:         string(call.Status),
		RiskLevel:      string(call.RiskLevel),
		ApprovalStatus: string(call.ApprovalStatus),
		Result:         call.Result,
		ErrorCode:      call.ErrorCode,
		ErrorMessage:   call.ErrorMessage,
	}
}

// encodeEventValue 将事件输入输出编码为 JSON RawMessage。
func encodeEventValue(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage(`null`), nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		return raw, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

// firstString 读取 map 中第一个字符串字段，缺失时返回默认值。
func firstString(values map[string]any, key string, fallback string) string {
	value, ok := values[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

// stringPtr 返回字符串指针。
func stringPtr(value string) *string {
	return &value
}

// mapWriteError 将数据库写入错误映射为统一应用错误。
func mapWriteError(message string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apperrors.Wrap(apperrors.CodeConflict, message, err)
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, message, err)
}
