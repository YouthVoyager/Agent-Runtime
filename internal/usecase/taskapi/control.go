package taskapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domainevent "stableagent/internal/domain/event"
	domaintask "stableagent/internal/domain/task"
	"stableagent/internal/infra/postgres/db"
	"stableagent/internal/observability/tracing"
	"stableagent/internal/security/authn"
	"stableagent/internal/security/authz"
	"stableagent/internal/usecase/auditlog"
	apperrors "stableagent/pkg/errors"
	"stableagent/pkg/ids"
	"stableagent/pkg/jsonx"
)

type CancelTaskInput struct {
	TenantID  string
	UserID    string
	UserRole  string
	TaskID    string
	IP        string
	UserAgent string
}

type ResumeTaskInput struct {
	TenantID  string
	UserID    string
	UserRole  string
	TaskID    string
	IP        string
	UserAgent string
}

// PauseTaskInput 描述暂停任务请求,契约 §2 仅 owner/tenant_admin/platform_admin 可操作。
type PauseTaskInput struct {
	TenantID  string
	UserID    string
	UserRole  string
	TaskID    string
	IP        string
	UserAgent string
}

// ResumeFromCheckpointInput 描述从指定 checkpoint 恢复任务的请求。
type ResumeFromCheckpointInput struct {
	TenantID     string
	UserID       string
	UserRole     string
	TaskID       string
	CheckpointID string
	IP           string
	UserAgent    string
}

// GetTaskStateInput 描述查询任务当前 AgentState 的请求。
type GetTaskStateInput struct {
	TenantID string
	UserID   string
	UserRole string
	TaskID   string
}

// TaskStateResult 是任务当前 AgentState 的 API 响应,角色分级返回 + 脱敏由用例层完成。
type TaskStateResult struct {
	TaskID           string          `json:"task_id"`
	Version          int32           `json:"version"`
	CurrentPlan      json.RawMessage `json:"current_plan"`
	CurrentStep      int32           `json:"current_step"`
	MemorySummary    string          `json:"memory_summary"`
	Constraints      json.RawMessage `json:"constraints"`
	Artifacts        json.RawMessage `json:"artifacts"`
	LoopFingerprints json.RawMessage `json:"loop_fingerprints"`
	State            json.RawMessage `json:"state,omitempty"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type TaskStatusResult struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

type ResumeTaskResult struct {
	TaskID                 string `json:"task_id"`
	Status                 string `json:"status"`
	ResumeFromCheckpointID string `json:"resume_from_checkpoint_id"`
}

type DecideToolCallInput struct {
	TenantID       string
	UserID         string
	UserRole       string
	CallID         string
	Approve        bool
	Comment        string
	Note           string
	Reason         string
	TerminateTask  bool
	IP             string
	UserAgent      string
}

type ToolCallDecisionResult struct {
	CallID         string `json:"call_id"`
	ApprovalStatus string `json:"approval_status"`
	ToolCallStatus string `json:"tool_call_status"`
	TaskStatus     string `json:"task_status"`
}

type ListTaskChildrenInput struct {
	TenantID string
	UserID   string
	UserRole string
	TaskID   string
	Limit    int
	Offset   int
}

type ToolCallItem struct {
	CallID         string          `json:"call_id"`
	TaskID         string          `json:"task_id"`
	ToolName       string          `json:"tool_name"`
	Status         string          `json:"status"`
	RiskLevel      string          `json:"risk_level"`
	ApprovalStatus string          `json:"approval_status"`
	Result         json.RawMessage `json:"result,omitempty"`
	ErrorCode      *string         `json:"error_code,omitempty"`
	ErrorMessage   *string         `json:"error_message,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type CheckpointItem struct {
	CheckpointID  string          `json:"checkpoint_id"`
	TaskID        string          `json:"task_id"`
	StateVersion  int32           `json:"state_version"`
	EventOffset   int64           `json:"event_offset"`
	StateSnapshot json.RawMessage `json:"state_snapshot"`
	Reason        string          `json:"reason"`
	TraceID       *string         `json:"trace_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type ArtifactItem struct {
	ArtifactID     string          `json:"artifact_id"`
	TaskID         string          `json:"task_id"`
	Type           string          `json:"type"`
	Name           string          `json:"name"`
	MediaType      *string         `json:"media_type,omitempty"`
	StorageBackend string          `json:"storage_backend"`
	StorageKey     *string         `json:"storage_key,omitempty"`
	ContentJSON    json.RawMessage `json:"content_json,omitempty"`
	SizeBytes      int64           `json:"size_bytes"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
}

type ToolCallPage struct {
	Items []ToolCallItem `json:"items"`
}

type CheckpointPage struct {
	Items []CheckpointItem `json:"items"`
}

type ArtifactPage struct {
	Items []ArtifactItem `json:"items"`
}

// CancelTask 将任务标记为取消中，并写入 Redis cancel flag。
func (s *Service) CancelTask(ctx context.Context, input CancelTaskInput) (TaskStatusResult, error) {
	task, err := s.authorizeControlAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID)
	if err != nil {
		return TaskStatusResult{}, err
	}
	status := domaintask.Status(task.Status)
	if isTerminalStatus(status) {
		return TaskStatusResult{TaskID: task.TaskID, Status: string(status)}, nil
	}
	if s.cancelStore != nil {
		if err := s.cancelStore.Set(ctx, input.TaskID); err != nil {
			return TaskStatusResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "写入取消标记失败", err)
		}
	}
	updated, err := s.queries.UpdateAgentTaskStatus(ctx, db.UpdateAgentTaskStatusParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
		Status:   db.TaskStatusCANCELING,
	})
	if err != nil {
		return TaskStatusResult{}, mapPostgresWriteError(err)
	}
	s.audit(ctx, auditlog.LogInput{
		ActorID:      input.UserID,
		TenantID:     input.TenantID,
		Action:       "task.cancel",
		ResourceType: "task",
		ResourceID:   input.TaskID,
		Before:       map[string]any{"status": string(task.Status)},
		After:        map[string]any{"status": string(updated.Status)},
		IP:           input.IP,
		UserAgent:    input.UserAgent,
	})
	return TaskStatusResult{TaskID: updated.TaskID, Status: domaintask.StatusForAPI(string(updated.Status))}, nil
}

// PauseTask 将 RUNNING 任务暂停:写入 Redis 暂停标记并原子切换状态为 PAUSED,
// Worker 会在下一个 step 边界读取该标记安全停下,不会覆盖已经落地的 PAUSED 状态。
func (s *Service) PauseTask(ctx context.Context, input PauseTaskInput) (TaskStatusResult, error) {
	task, err := s.authorizeControlAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID)
	if err != nil {
		return TaskStatusResult{}, err
	}
	if domaintask.Status(task.Status) != domaintask.StatusRunning {
		return TaskStatusResult{}, apperrors.New(apperrors.CodeConflict, "只有 RUNNING 状态的任务可以暂停")
	}
	if s.pauseStore != nil {
		if err := s.pauseStore.Set(ctx, input.TaskID); err != nil {
			return TaskStatusResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "写入暂停标记失败", err)
		}
	}
	updated, err := s.queries.UpdateAgentTaskStatusIfCurrent(ctx, db.UpdateAgentTaskStatusIfCurrentParams{
		TenantID:      input.TenantID,
		TaskID:        input.TaskID,
		NextStatus:    db.TaskStatusPAUSED,
		CurrentStatus: db.TaskStatusRUNNING,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TaskStatusResult{}, apperrors.New(apperrors.CodeConflict, "任务状态已变化,暂停失败")
		}
		return TaskStatusResult{}, mapPostgresWriteError(err)
	}
	pausedEvent, err := appendEventWithQueries(ctx, s.queries, appendEventInput{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
		UserID:   input.UserID,
		Source:   "api-service",
		Type:     domainevent.TypeTaskPaused,
		Input:    map[string]any{"previous_status": task.Status},
		Output:   map[string]any{"status": updated.Status},
		TraceID:  updated.TraceID,
	})
	if err == nil {
		s.notifyPersistedEvents(ctx, pausedEvent)
	}
	s.audit(ctx, auditlog.LogInput{
		ActorID:      input.UserID,
		TenantID:     input.TenantID,
		Action:       "task.pause",
		ResourceType: "task",
		ResourceID:   input.TaskID,
		Before:       map[string]any{"status": string(task.Status)},
		After:        map[string]any{"status": string(updated.Status)},
		IP:           input.IP,
		UserAgent:    input.UserAgent,
	})
	return TaskStatusResult{TaskID: updated.TaskID, Status: domaintask.StatusForAPI(string(updated.Status))}, nil
}

// ResumeTask 从最近 checkpoint 重新排队任务。
func (s *Service) ResumeTask(ctx context.Context, input ResumeTaskInput) (ResumeTaskResult, error) {
	task, err := s.authorizeControlAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID)
	if err != nil {
		return ResumeTaskResult{}, err
	}
	status := domaintask.Status(task.Status)
	if status == domaintask.StatusQueued || status == domaintask.StatusRunning {
		latest, _ := s.queries.GetLatestCheckpoint(ctx, db.GetLatestCheckpointParams{TenantID: input.TenantID, TaskID: input.TaskID})
		return ResumeTaskResult{TaskID: task.TaskID, Status: string(status), ResumeFromCheckpointID: latest.CheckpointID}, nil
	}
	if status == domaintask.StatusWaitingApproval {
		return ResumeTaskResult{}, apperrors.New(apperrors.CodeConflict, "等待审批的任务需要先处理审批")
	}
	// 设计方案 §5.4:resume 仅允许 FAILED / PAUSED / STOPPED_BY_LIMIT(含 RETRYABLE_FAILED)。
	if !domaintask.CanResume(status) {
		return ResumeTaskResult{}, apperrors.New(apperrors.CodeConflict, "当前状态不允许恢复任务")
	}
	checkpoint, err := s.queries.GetLatestCheckpoint(ctx, db.GetLatestCheckpointParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResumeTaskResult{}, apperrors.New(apperrors.CodeConflict, "任务没有可恢复 checkpoint")
		}
		return ResumeTaskResult{}, mapPostgresReadError(err)
	}
	result, err := s.resumeToCheckpoint(ctx, task, input.UserID, checkpoint)
	if err != nil {
		return ResumeTaskResult{}, err
	}
	s.audit(ctx, auditlog.LogInput{
		ActorID:      input.UserID,
		TenantID:     input.TenantID,
		Action:       "task.resume",
		ResourceType: "task",
		ResourceID:   input.TaskID,
		Before:       map[string]any{"status": string(task.Status)},
		After:        map[string]any{"status": result.Status, "resume_from_checkpoint_id": result.ResumeFromCheckpointID},
		IP:           input.IP,
		UserAgent:    input.UserAgent,
	})
	return result, nil
}

// ResumeFromCheckpoint 从调用方指定的 checkpoint 恢复任务,校验 checkpoint 属于该任务后复用统一恢复链路。
func (s *Service) ResumeFromCheckpoint(ctx context.Context, input ResumeFromCheckpointInput) (ResumeTaskResult, error) {
	task, err := s.authorizeControlAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID)
	if err != nil {
		return ResumeTaskResult{}, err
	}
	status := domaintask.Status(task.Status)
	if status == domaintask.StatusWaitingApproval {
		return ResumeTaskResult{}, apperrors.New(apperrors.CodeConflict, "等待审批的任务需要先处理审批")
	}
	if !domaintask.CanResume(status) {
		return ResumeTaskResult{}, apperrors.New(apperrors.CodeConflict, "当前状态不允许恢复任务")
	}
	checkpointID := strings.TrimSpace(input.CheckpointID)
	if checkpointID == "" {
		return ResumeTaskResult{}, apperrors.New(apperrors.CodeInvalidArg, "checkpoint_id 不能为空")
	}
	checkpoint, err := s.queries.GetCheckpoint(ctx, db.GetCheckpointParams{
		TenantID:     input.TenantID,
		CheckpointID: checkpointID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResumeTaskResult{}, apperrors.New(apperrors.CodeNotFound, "checkpoint 不存在")
		}
		return ResumeTaskResult{}, mapPostgresReadError(err)
	}
	// 校验 checkpoint 归属:必须属于当前任务,避免跨任务恢复导致状态错乱。
	if checkpoint.TaskID != input.TaskID {
		return ResumeTaskResult{}, apperrors.New(apperrors.CodeInvalidArg, "checkpoint 不属于该任务")
	}
	result, err := s.resumeToCheckpoint(ctx, task, input.UserID, checkpoint)
	if err != nil {
		return ResumeTaskResult{}, err
	}
	s.audit(ctx, auditlog.LogInput{
		ActorID:      input.UserID,
		TenantID:     input.TenantID,
		Action:       "task.resume_from_checkpoint",
		ResourceType: "task",
		ResourceID:   input.TaskID,
		Before:       map[string]any{"status": string(task.Status)},
		After:        map[string]any{"status": result.Status, "resume_from_checkpoint_id": result.ResumeFromCheckpointID},
		IP:           input.IP,
		UserAgent:    input.UserAgent,
	})
	return result, nil
}

// resumeToCheckpoint 是 ResumeTask 和 ResumeFromCheckpoint 共用的恢复链路:
// 任务重新排队为 QUEUED、写 START_WORKFLOW outbox、追加 TASK_RESUMED 事件、清理暂停标记。
func (s *Service) resumeToCheckpoint(ctx context.Context, task db.AgentTask, userID string, checkpoint db.Checkpoint) (ResumeTaskResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ResumeTaskResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "创建恢复事务失败", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	queries := db.New(tx)
	updated, err := queries.UpdateAgentTaskStatus(ctx, db.UpdateAgentTaskStatusParams{
		TenantID: task.TenantID,
		TaskID:   task.TaskID,
		Status:   db.TaskStatusQUEUED,
	})
	if err != nil {
		return ResumeTaskResult{}, mapPostgresWriteError(err)
	}
	if _, err := createStartWorkflowOutbox(ctx, queries, task.TenantID, task.TaskID, userID, updated.TraceID); err != nil {
		return ResumeTaskResult{}, err
	}
	resumedEvent, err := appendEventWithQueries(ctx, queries, appendEventInput{
		TenantID: task.TenantID,
		TaskID:   task.TaskID,
		UserID:   userID,
		Source:   "api-service",
		Type:     domainevent.TypeTaskResumed,
		Input:    map[string]any{"checkpoint_id": checkpoint.CheckpointID},
		Output:   map[string]any{"status": updated.Status},
		TraceID:  updated.TraceID,
	})
	if err != nil {
		return ResumeTaskResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ResumeTaskResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "提交恢复事务失败", err)
	}
	if s.pauseStore != nil {
		// 恢复成功后清理暂停标记,避免 Worker 误判任务仍处于暂停状态。
		_ = s.pauseStore.Clear(ctx, task.TaskID)
	}
	s.notifyPersistedEvents(ctx, resumedEvent)
	return ResumeTaskResult{
		TaskID:                 updated.TaskID,
		Status:                 domaintask.StatusForAPI(string(updated.Status)),
		ResumeFromCheckpointID: checkpoint.CheckpointID,
	}, nil
}

// DecideToolCall 审批或拒绝工具调用，并按结果重新排队或失败任务。
func (s *Service) DecideToolCall(ctx context.Context, input DecideToolCallInput) (ToolCallDecisionResult, error) {
	ctx, span := tracing.StartSpan(ctx, "approval.decide", "", input.TenantID)
	defer span.End()
	call, err := s.queries.GetToolCall(ctx, db.GetToolCallParams{
		TenantID: input.TenantID,
		CallID:   input.CallID,
	})
	if err != nil {
		return ToolCallDecisionResult{}, mapPostgresReadError(err)
	}
	task, err := s.queries.GetAgentTask(ctx, db.GetAgentTaskParams{
		TenantID: input.TenantID,
		TaskID:   call.TaskID,
	})
	if err != nil {
		return ToolCallDecisionResult{}, mapPostgresReadError(err)
	}
	// 契约 §3:审批动作仅 approver / tenant_admin / platform_admin 可操作,任务所有者本人不能自我审批。
	if !canDecideApproval(authn.User{TenantID: input.TenantID, UserID: input.UserID, Role: input.UserRole}) {
		return ToolCallDecisionResult{}, apperrors.New(apperrors.CodeForbidden, "没有审批工具调用的权限")
	}
	approval, err := s.queries.GetApprovalByCall(ctx, db.GetApprovalByCallParams{
		TenantID: input.TenantID,
		CallID:   input.CallID,
	})
	if err != nil {
		return ToolCallDecisionResult{}, mapPostgresReadError(err)
	}
	// note 为 approve 扩展字段,reason 为 reject 扩展字段,兼容旧版通用 comment 字段。
	comment := firstNonBlankString(input.Comment, input.Note, input.Reason)
	targetApproval := db.ApprovalStatusREJECTED
	targetToolStatus := db.ToolCallStatusFAILED
	targetTaskStatus := db.TaskStatusFAILED
	eventOutput := map[string]any{"decision": "REJECTED"}
	if input.Approve {
		targetApproval = db.ApprovalStatusAPPROVED
		targetToolStatus = db.ToolCallStatusPENDING
		targetTaskStatus = db.TaskStatusQUEUED
		eventOutput = map[string]any{"decision": "APPROVED"}
	} else if !input.TerminateTask {
		// terminate_task=false:仅拒绝该工具调用,任务重新排队等待 Worker 继续推进,不直接失败任务。
		targetTaskStatus = db.TaskStatusQUEUED
	}
	if approval.Status != db.ApprovalStatusPENDING {
		if approval.Status == targetApproval {
			return ToolCallDecisionResult{
				CallID:         call.CallID,
				ApprovalStatus: string(approval.Status),
				ToolCallStatus: string(call.Status),
				TaskStatus:     domaintask.StatusForAPI(string(task.Status)),
			}, nil
		}
		return ToolCallDecisionResult{}, apperrors.New(apperrors.CodeConflict, "审批已经被其他决策处理")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ToolCallDecisionResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "创建审批事务失败", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	queries := db.New(tx)
	updatedApproval, err := decideApprovalByCall(ctx, tx, input.TenantID, input.CallID, input.UserID, comment, targetApproval)
	if err != nil {
		return ToolCallDecisionResult{}, err
	}
	updatedCall, err := updateToolCallAfterDecision(ctx, tx, input.TenantID, input.CallID, targetToolStatus, targetApproval, !input.Approve)
	if err != nil {
		return ToolCallDecisionResult{}, err
	}
	updatedTask, err := queries.UpdateAgentTaskStatus(ctx, db.UpdateAgentTaskStatusParams{
		TenantID: input.TenantID,
		TaskID:   call.TaskID,
		Status:   targetTaskStatus,
	})
	if err != nil {
		return ToolCallDecisionResult{}, mapPostgresWriteError(err)
	}
	if targetTaskStatus == db.TaskStatusQUEUED {
		// 审批通过,或拒绝但 terminate_task=false,任务都需要重新排队交给 Worker 继续推进。
		if _, err := createStartWorkflowOutbox(ctx, queries, input.TenantID, call.TaskID, task.UserID, task.TraceID); err != nil {
			return ToolCallDecisionResult{}, err
		}
	}
	approvalEvent, err := appendEventWithQueries(ctx, queries, appendEventInput{
		TenantID: input.TenantID,
		TaskID:   call.TaskID,
		UserID:   input.UserID,
		Source:   "api-service",
		Type:     domainevent.TypeToolApprovalDecided,
		Input:    map[string]any{"call_id": call.CallID, "approval_id": updatedApproval.ApprovalID},
		Output:   eventOutput,
		TraceID:  task.TraceID,
	})
	if err != nil {
		return ToolCallDecisionResult{}, err
	}
	var failedEvent *db.AgentEvent
	if !input.Approve && input.TerminateTask {
		row, err := appendEventWithQueries(ctx, queries, appendEventInput{
			TenantID: input.TenantID,
			TaskID:   call.TaskID,
			UserID:   input.UserID,
			Source:   "api-service",
			Type:     domainevent.TypeTaskFailed,
			Input:    map[string]any{"call_id": call.CallID},
			Output:   map[string]any{"reason": "tool approval rejected"},
			TraceID:  task.TraceID,
		})
		if err != nil {
			return ToolCallDecisionResult{}, err
		}
		failedEvent = &row
	}
	if err := tx.Commit(ctx); err != nil {
		return ToolCallDecisionResult{}, apperrors.Wrap(apperrors.CodeUnavailable, "提交审批事务失败", err)
	}
	s.notifyPersistedEvents(ctx, approvalEvent)
	if failedEvent != nil {
		s.notifyPersistedEvents(ctx, *failedEvent)
	}
	action := "approval.reject"
	if input.Approve {
		action = "approval.approve"
	}
	s.audit(ctx, auditlog.LogInput{
		ActorID:      input.UserID,
		TenantID:     input.TenantID,
		Action:       action,
		ResourceType: "tool_call",
		ResourceID:   call.CallID,
		Before:       map[string]any{"approval_status": string(approval.Status)},
		After:        map[string]any{"approval_status": string(updatedApproval.Status), "comment": comment, "terminate_task": input.TerminateTask},
		IP:           input.IP,
		UserAgent:    input.UserAgent,
	})
	return ToolCallDecisionResult{
		CallID:         updatedCall.CallID,
		ApprovalStatus: string(updatedApproval.Status),
		ToolCallStatus: string(updatedCall.Status),
		TaskStatus:     domaintask.StatusForAPI(string(updatedTask.Status)),
	}, nil
}

// canDecideApproval 判断用户是否具备审批工具调用的权限:approver / tenant_admin / platform_admin。
func canDecideApproval(user authn.User) bool {
	return authz.IsApprover(user) || authz.IsAdmin(user)
}

// firstNonBlankString 返回第一个去除首尾空白后非空的字符串。
func firstNonBlankString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// notifyPersistedEvents 在事务提交后通知 SSE hub 和跨实例事件总线。
func (s *Service) notifyPersistedEvents(ctx context.Context, events ...db.AgentEvent) {
	if s.eventNotifier == nil {
		return
	}
	for _, event := range events {
		s.eventNotifier.NotifyPersisted(ctx, eventFromCreatedRow(event))
	}
}

// ListToolCalls 查询任务下的工具调用。
func (s *Service) ListToolCalls(ctx context.Context, input ListTaskChildrenInput) (ToolCallPage, error) {
	if _, err := s.authorizeTaskAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID); err != nil {
		return ToolCallPage{}, err
	}
	limit, offset := normalizeChildPagination(input.Limit, input.Offset)
	rows, err := s.queries.ListToolCallsByTask(ctx, db.ListToolCallsByTaskParams{
		TenantID:   input.TenantID,
		TaskID:     input.TaskID,
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return ToolCallPage{}, mapPostgresReadError(err)
	}
	items := make([]ToolCallItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toolCallItemFromDB(row))
	}
	return ToolCallPage{Items: items}, nil
}

// ListCheckpoints 查询任务 checkpoint 列表。
func (s *Service) ListCheckpoints(ctx context.Context, input ListTaskChildrenInput) (CheckpointPage, error) {
	if _, err := s.authorizeTaskAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID); err != nil {
		return CheckpointPage{}, err
	}
	limit, offset := normalizeChildPagination(input.Limit, input.Offset)
	rows, err := s.queries.ListCheckpointsByTask(ctx, db.ListCheckpointsByTaskParams{
		TenantID:   input.TenantID,
		TaskID:     input.TaskID,
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return CheckpointPage{}, mapPostgresReadError(err)
	}
	items := make([]CheckpointItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, checkpointItemFromDB(row))
	}
	return CheckpointPage{Items: items}, nil
}

// ListArtifacts 查询任务 artifact 列表。
func (s *Service) ListArtifacts(ctx context.Context, input ListTaskChildrenInput) (ArtifactPage, error) {
	if _, err := s.authorizeTaskAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID); err != nil {
		return ArtifactPage{}, err
	}
	limit, offset := normalizeChildPagination(input.Limit, input.Offset)
	rows, err := s.queries.ListArtifactsByTask(ctx, db.ListArtifactsByTaskParams{
		TenantID:   input.TenantID,
		TaskID:     input.TaskID,
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return ArtifactPage{}, mapPostgresReadError(err)
	}
	items := make([]ArtifactItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, artifactItemFromDB(row))
	}
	return ArtifactPage{Items: items}, nil
}

// authorizeTaskAccess 查询任务并校验当前用户读取或控制权限。
func (s *Service) authorizeTaskAccess(ctx context.Context, tenantID string, userID string, userRole string, taskID string) (db.AgentTask, error) {
	if strings.TrimSpace(taskID) == "" {
		return db.AgentTask{}, apperrors.New(apperrors.CodeInvalidArg, "task_id 不能为空")
	}
	task, err := s.queries.GetAgentTask(ctx, db.GetAgentTaskParams{
		TenantID: tenantID,
		TaskID:   taskID,
	})
	if err != nil {
		return db.AgentTask{}, mapPostgresReadError(err)
	}
	if !authz.CanReadTask(authn.User{TenantID: tenantID, UserID: userID, Role: userRole}, task.UserID) {
		return db.AgentTask{}, apperrors.New(apperrors.CodeForbidden, "没有访问该任务的权限")
	}
	return task, nil
}

// authorizeControlAccess 校验任务控制类操作(cancel/pause/resume/resume-from-checkpoint)权限。
// 契约 §2:仅 owner / tenant_admin / platform_admin 可操作,auditor 全局只读一律拒绝写操作。
func (s *Service) authorizeControlAccess(ctx context.Context, tenantID string, userID string, userRole string, taskID string) (db.AgentTask, error) {
	task, err := s.authorizeTaskAccess(ctx, tenantID, userID, userRole, taskID)
	if err != nil {
		return db.AgentTask{}, err
	}
	user := authn.User{TenantID: tenantID, UserID: userID, Role: userRole}
	if authz.IsAuditor(user) {
		return db.AgentTask{}, apperrors.New(apperrors.CodeForbidden, "审计员没有任务控制权限")
	}
	return task, nil
}

// GetTaskState 查询任务当前 AgentState,按角色分级返回并对敏感字段脱敏。
// user 角色只返回摘要字段;tenant_admin/platform_admin/auditor 额外返回完整 state JSON。
func (s *Service) GetTaskState(ctx context.Context, input GetTaskStateInput) (TaskStateResult, error) {
	if _, err := s.authorizeTaskAccess(ctx, input.TenantID, input.UserID, input.UserRole, input.TaskID); err != nil {
		return TaskStateResult{}, err
	}
	state, err := s.queries.GetAgentState(ctx, db.GetAgentStateParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
	})
	if err != nil {
		return TaskStateResult{}, mapPostgresReadError(err)
	}
	memorySummary := ""
	if state.MemorySummary != nil {
		memorySummary = *state.MemorySummary
	}
	result := TaskStateResult{
		TaskID:           input.TaskID,
		Version:          state.Version,
		CurrentPlan:      jsonx.RedactJSON(state.CurrentPlan),
		CurrentStep:      state.CurrentStep,
		MemorySummary:    memorySummary,
		Constraints:       jsonx.RedactJSON(state.Constraints),
		Artifacts:        jsonx.RedactJSON(state.Artifacts),
		LoopFingerprints: state.LoopFingerprints,
		UpdatedAt:        pgTime(state.UpdatedAt),
	}
	user := authn.User{TenantID: input.TenantID, UserID: input.UserID, Role: input.UserRole}
	if authz.IsAdmin(user) || authz.IsAuditor(user) {
		full, err := json.Marshal(map[string]any{
			"current_plan":      json.RawMessage(jsonx.RedactJSON(state.CurrentPlan)),
			"current_step":      state.CurrentStep,
			"memory_summary":    memorySummary,
			"constraints":       json.RawMessage(jsonx.RedactJSON(state.Constraints)),
			"artifacts":         json.RawMessage(jsonx.RedactJSON(state.Artifacts)),
			"loop_fingerprints": json.RawMessage(state.LoopFingerprints),
			"version":           state.Version,
		})
		if err == nil {
			result.State = full
		}
	}
	return result, nil
}

// createStartWorkflowOutbox 写入重新启动本地 workflow 的 outbox 消息。
func createStartWorkflowOutbox(ctx context.Context, queries *db.Queries, tenantID string, taskID string, userID string, traceID *string) (db.TaskOutbox, error) {
	payload, err := json.Marshal(map[string]any{
		"task_id":   taskID,
		"tenant_id": tenantID,
		"user_id":   userID,
		"trace_id":  traceID,
	})
	if err != nil {
		return db.TaskOutbox{}, apperrors.Wrap(apperrors.CodeInternal, "序列化 outbox payload 失败", err)
	}
	message, err := queries.CreateTaskOutboxMessage(ctx, db.CreateTaskOutboxMessageParams{
		TenantID:      tenantID,
		AggregateType: "agent_task",
		AggregateID:   taskID,
		EventType:     outboxStartWorkflow,
		Payload:       payload,
		Status:        db.OutboxStatusPENDING,
		NextRetryAt:   pgtype.Timestamptz{},
	})
	if err != nil {
		return db.TaskOutbox{}, mapPostgresWriteError(err)
	}
	return message, nil
}

type appendEventInput struct {
	TenantID string
	TaskID   string
	UserID   string
	Source   string
	Type     domainevent.Type
	Input    any
	Output   any
	TraceID  *string
}

// appendEventWithQueries 在调用方事务内写入 AgentEvent。
func appendEventWithQueries(ctx context.Context, queries *db.Queries, input appendEventInput) (db.AgentEvent, error) {
	eventInput, err := marshalEventValue(input.Input)
	if err != nil {
		return db.AgentEvent{}, err
	}
	eventOutput, err := marshalEventValue(input.Output)
	if err != nil {
		return db.AgentEvent{}, err
	}
	spanID, err := ids.New("span")
	if err != nil {
		return db.AgentEvent{}, apperrors.Wrap(apperrors.CodeInternal, "生成 span_id 失败", err)
	}
	metadata, err := domainevent.MarshalMetadata(domainevent.Metadata{
		UserID: input.UserID,
		Source: input.Source,
	})
	if err != nil {
		return db.AgentEvent{}, apperrors.Wrap(apperrors.CodeInternal, "序列化事件 metadata 失败", err)
	}
	row, err := queries.CreateAgentEvent(ctx, db.CreateAgentEventParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
		Type:     string(input.Type),
		Input:    eventInput,
		Output:   eventOutput,
		Metadata: metadata,
		TraceID:  input.TraceID,
		SpanID:   &spanID,
	})
	if err != nil {
		return db.AgentEvent{}, mapPostgresWriteError(err)
	}
	return row, nil
}

// decideApprovalByCall 在事务内按 call_id 决策 pending 审批。
func decideApprovalByCall(ctx context.Context, tx pgx.Tx, tenantID string, callID string, approverID string, comment string, status db.ApprovalStatus) (db.Approval, error) {
	row := tx.QueryRow(ctx, `
update approvals
set status = $1,
    approver_id = $2,
    comment = nullif($3, ''),
    decided_at = now()
where tenant_id = $4
  and call_id = $5
  and status = 'PENDING'
returning approval_id, task_id, tenant_id, call_id, approver_id, status, risk_level, approval_reason, comment, expires_at, decided_at, created_at`, status, approverID, strings.TrimSpace(comment), tenantID, callID)
	var approval db.Approval
	err := row.Scan(
		&approval.ApprovalID,
		&approval.TaskID,
		&approval.TenantID,
		&approval.CallID,
		&approval.ApproverID,
		&approval.Status,
		&approval.RiskLevel,
		&approval.ApprovalReason,
		&approval.Comment,
		&approval.ExpiresAt,
		&approval.DecidedAt,
		&approval.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Approval{}, apperrors.New(apperrors.CodeConflict, "审批已被处理")
		}
		return db.Approval{}, mapPostgresWriteError(err)
	}
	return approval, nil
}

// updateToolCallAfterDecision 在事务内同步工具调用审批状态。
func updateToolCallAfterDecision(ctx context.Context, tx pgx.Tx, tenantID string, callID string, status db.ToolCallStatus, approvalStatus db.ApprovalStatus, rejected bool) (db.ToolCall, error) {
	errorCode := (*string)(nil)
	errorMessage := (*string)(nil)
	if rejected {
		code := "TOOL_APPROVAL_REJECTED"
		message := "工具调用被人工拒绝"
		errorCode = &code
		errorMessage = &message
	}
	row := tx.QueryRow(ctx, `
update tool_calls
set status = $1,
    approval_status = $2,
    error_code = $3,
    error_message = $4,
    updated_at = now(),
    finished_at = case when $5 then now() else finished_at end
where tenant_id = $6
  and call_id = $7
returning call_id, task_id, tenant_id, user_id, tool_name, arguments, arguments_hash, idempotency_key, status,
          result, result_artifact_id, risk_level, approval_status, error_code, error_message, started_at, finished_at, created_at, updated_at`,
		status, approvalStatus, errorCode, errorMessage, rejected, tenantID, callID)
	var call db.ToolCall
	err := row.Scan(
		&call.CallID,
		&call.TaskID,
		&call.TenantID,
		&call.UserID,
		&call.ToolName,
		&call.Arguments,
		&call.ArgumentsHash,
		&call.IdempotencyKey,
		&call.Status,
		&call.Result,
		&call.ResultArtifactID,
		&call.RiskLevel,
		&call.ApprovalStatus,
		&call.ErrorCode,
		&call.ErrorMessage,
		&call.StartedAt,
		&call.FinishedAt,
		&call.CreatedAt,
		&call.UpdatedAt,
	)
	if err != nil {
		return db.ToolCall{}, mapPostgresWriteError(err)
	}
	return call, nil
}

// marshalEventValue 将任意事件值编码为 JSON。
func marshalEventValue(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage(`null`), nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		return raw, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "序列化事件 JSON 失败", err)
	}
	return json.RawMessage(data), nil
}

// normalizeChildPagination 规范化子资源分页参数。
func normalizeChildPagination(limit int, offset int) (int32, int32) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return int32(limit), int32(offset)
}

// isTerminalStatus 判断任务是否已经到达终态。
func isTerminalStatus(status domaintask.Status) bool {
	return status == domaintask.StatusCanceled || status == domaintask.StatusSucceeded || status == domaintask.StatusFailed
}

// toolCallItemFromDB 转换工具调用列表项。
func toolCallItemFromDB(row db.ToolCall) ToolCallItem {
	return ToolCallItem{
		CallID:         row.CallID,
		TaskID:         row.TaskID,
		ToolName:       row.ToolName,
		Status:         string(row.Status),
		RiskLevel:      string(row.RiskLevel),
		ApprovalStatus: string(row.ApprovalStatus),
		Result:         row.Result,
		ErrorCode:      row.ErrorCode,
		ErrorMessage:   row.ErrorMessage,
		CreatedAt:      pgTime(row.CreatedAt),
		UpdatedAt:      pgTime(row.UpdatedAt),
	}
}

// checkpointItemFromDB 转换 checkpoint 列表项。
func checkpointItemFromDB(row db.Checkpoint) CheckpointItem {
	return CheckpointItem{
		CheckpointID:  row.CheckpointID,
		TaskID:        row.TaskID,
		StateVersion:  row.StateVersion,
		EventOffset:   row.EventOffset,
		StateSnapshot: row.StateSnapshot,
		Reason:        row.Reason,
		TraceID:       row.TraceID,
		CreatedAt:     pgTime(row.CreatedAt),
	}
}

// artifactItemFromDB 转换 artifact 列表项。
func artifactItemFromDB(row db.Artifact) ArtifactItem {
	return ArtifactItem{
		ArtifactID:     row.ArtifactID,
		TaskID:         row.TaskID,
		Type:           string(row.Type),
		Name:           row.Name,
		MediaType:      row.MediaType,
		StorageBackend: row.StorageBackend,
		StorageKey:     row.StorageKey,
		ContentJSON:    row.ContentJson,
		SizeBytes:      row.SizeBytes,
		Status:         string(row.Status),
		CreatedAt:      pgTime(row.CreatedAt),
	}
}
