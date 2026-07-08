package runtimeworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	domainevent "stableagent/internal/domain/event"
	"stableagent/internal/domain/loopdetect"
	"stableagent/internal/domain/runtimeplan"
	domaintask "stableagent/internal/domain/task"
	domaintool "stableagent/internal/domain/toolcall"
	"stableagent/internal/infra/postgres/db"
	redisinfra "stableagent/internal/infra/redis"
	"stableagent/internal/observability/metrics"
	"stableagent/internal/observability/tracing"
	"stableagent/internal/usecase/eventapi"
	toolusecase "stableagent/internal/usecase/toolgateway"
	apperrors "stableagent/pkg/errors"
	"stableagent/pkg/ids"
)

type Service struct {
	pool         *pgxpool.Pool
	queries      *db.Queries
	events       *eventapi.Service
	cancelStore  *redisinfra.CancelStore
	httpClient   *http.Client
	llmURL       string
	toolURL      string
	pollInterval time.Duration
	logger       *slog.Logger
}

type Config struct {
	LLMGatewayURL  string
	ToolGatewayURL string
	PollInterval   time.Duration
}

// NewService 创建 Runtime Worker 本地 workflow 执行服务。
func NewService(pool *pgxpool.Pool, events *eventapi.Service, cancelStore *redisinfra.CancelStore, cfg Config, logger *slog.Logger) *Service {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		pool:         pool,
		queries:      db.New(pool),
		events:       events,
		cancelStore:  cancelStore,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		llmURL:       strings.TrimRight(cfg.LLMGatewayURL, "/"),
		toolURL:      strings.TrimRight(cfg.ToolGatewayURL, "/"),
		pollInterval: cfg.PollInterval,
		logger:       logger,
	}
}

// Start 启动 outbox 扫描和任务推进循环。
func (s *Service) Start(ctx context.Context) {
	go func() {
		s.tick(ctx)
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

// tick 执行一次 outbox 派发、可运行任务处理和审批过期扫描。
func (s *Service) tick(ctx context.Context) {
	if err := s.dispatchOutbox(ctx); err != nil {
		s.logger.Warn("outbox 派发失败", "error", err)
	}
	if err := s.processRunnableTasks(ctx); err != nil {
		s.logger.Warn("任务推进失败", "error", err)
	}
	if err := s.expireApprovals(ctx); err != nil {
		s.logger.Warn("审批过期扫描失败", "error", err)
	}
}

// expireApprovals 将超过 expires_at 仍未决策的 PENDING 审批置为 EXPIRED,
// 同步失败对应工具调用,等待审批的任务落 FAILED。
func (s *Service) expireApprovals(ctx context.Context) error {
	approvals, err := s.queries.ListExpiredPendingApprovals(ctx, 20)
	if err != nil {
		return err
	}
	for _, approval := range approvals {
		// 带 status='PENDING' 条件的条件更新保证与人工决策互斥,并发时跳过。
		if _, err := s.queries.ExpireApproval(ctx, db.ExpireApprovalParams{
			TenantID:   approval.TenantID,
			ApprovalID: approval.ApprovalID,
		}); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				s.logger.Warn("过期审批失败", "approval_id", approval.ApprovalID, "error", err)
			}
			continue
		}
		errorCode := "TOOL_APPROVAL_EXPIRED"
		errorMessage := "审批超时未决策,已自动过期"
		if _, err := s.queries.FailToolCall(ctx, db.FailToolCallParams{
			TenantID:     approval.TenantID,
			CallID:       approval.CallID,
			ErrorCode:    &errorCode,
			ErrorMessage: &errorMessage,
		}); err != nil {
			s.logger.Warn("过期审批同步工具调用失败", "call_id", approval.CallID, "error", err)
			continue
		}
		if _, err := s.queries.UpdateToolCallApprovalStatus(ctx, db.UpdateToolCallApprovalStatusParams{
			TenantID:       approval.TenantID,
			CallID:         approval.CallID,
			ApprovalStatus: db.ApprovalStatusEXPIRED,
		}); err != nil {
			s.logger.Warn("过期审批更新工具审批状态失败", "call_id", approval.CallID, "error", err)
		}
		task, err := s.queries.GetAgentTask(ctx, db.GetAgentTaskParams{
			TenantID: approval.TenantID,
			TaskID:   approval.TaskID,
		})
		if err != nil {
			continue
		}
		if task.Status == db.TaskStatusWAITINGAPPROVAL {
			if err := s.failTask(ctx, task, errorCode, errorMessage); err != nil {
				s.logger.Warn("过期审批失败任务失败", "task_id", task.TaskID, "error", err)
			}
		}
		_ = s.appendEvent(ctx, task, domainevent.TypeToolApprovalDecided,
			map[string]any{"call_id": approval.CallID, "approval_id": approval.ApprovalID},
			map[string]any{"decision": "EXPIRED"})
		metrics.ApprovalsExpiredTotal.Inc()
	}
	return nil
}

// outboxStaleAfter 定义 PROCESSING 消息多久没有更新即视为死信,重置为 FAILED 等待重试。
const outboxStaleAfter = 30 * time.Second

// dispatchOutbox 扫描 START_WORKFLOW 消息并回填 workflow_id。
func (s *Service) dispatchOutbox(ctx context.Context) error {
	// worker 在 MarkTaskOutboxProcessing 之后崩溃会把消息留在 PROCESSING,先重置超时死信再派发。
	if err := s.queries.ResetStaleTaskOutboxProcessing(ctx, pgtype.Interval{
		Microseconds: outboxStaleAfter.Microseconds(),
		Valid:        true,
	}); err != nil {
		s.logger.Warn("重置超时 outbox 消息失败", "error", err)
	}
	messages, err := s.queries.ListPendingTaskOutboxMessages(ctx, 20)
	if err != nil {
		return err
	}
	for _, message := range messages {
		if message.EventType != "START_WORKFLOW" {
			continue
		}
		processing, err := s.queries.MarkTaskOutboxProcessing(ctx, message.OutboxID)
		if err != nil {
			continue
		}
		if err := s.markWorkflowStarted(ctx, processing); err != nil {
			_, _ = s.queries.MarkTaskOutboxFailed(ctx, db.MarkTaskOutboxFailedParams{
				Status:      db.OutboxStatusFAILED,
				NextRetryAt: pgtype.Timestamptz{Time: time.Now().Add(5 * time.Second), Valid: true},
				OutboxID:    processing.OutboxID,
			})
			s.logger.Warn("处理 START_WORKFLOW outbox 失败", "outbox_id", processing.OutboxID, "error", err)
			metrics.OutboxDispatchTotal.WithLabelValues("failed").Inc()
			continue
		}
		_, _ = s.queries.MarkTaskOutboxSent(ctx, processing.OutboxID)
		metrics.OutboxDispatchTotal.WithLabelValues("sent").Inc()
	}
	return nil
}

// markWorkflowStarted 将 task_id 作为本地 workflow_id 写回任务。
func (s *Service) markWorkflowStarted(ctx context.Context, message db.TaskOutbox) error {
	var payload struct {
		TaskID  string  `json:"task_id"`
		TraceID *string `json:"trace_id"`
	}
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return err
	}
	workflowID := payload.TaskID
	if workflowID == "" {
		workflowID = message.AggregateID
	}
	_, err := s.pool.Exec(ctx, `
update agent_tasks
set workflow_id = coalesce(workflow_id, $1),
    trace_id = coalesce(trace_id, $2),
    updated_at = now()
where tenant_id = $3
  and task_id = $4`, workflowID, payload.TraceID, message.TenantID, workflowID)
	return err
}

// processRunnableTasks 查询并推进 QUEUED/CANCELING 任务。
func (s *Service) processRunnableTasks(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
select task_id, tenant_id, user_id, goal, status, budget, budget_usage, workflow_id, trace_id,
       last_error_code, last_error_message, created_at, updated_at
from agent_tasks
where status in ('QUEUED', 'CANCELING')
order by updated_at asc, task_id asc
limit 10`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var tasks []db.AgentTask
	for rows.Next() {
		var task db.AgentTask
		if err := rows.Scan(
			&task.TaskID,
			&task.TenantID,
			&task.UserID,
			&task.Goal,
			&task.Status,
			&task.Budget,
			&task.BudgetUsage,
			&task.WorkflowID,
			&task.TraceID,
			&task.LastErrorCode,
			&task.LastErrorMessage,
			&task.CreatedAt,
			&task.UpdatedAt,
		); err != nil {
			return err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Status == db.TaskStatusCANCELING || s.cancelStore.IsCanceled(ctx, task.TaskID) {
			if err := s.finishCanceledTask(ctx, task); err != nil {
				s.logger.Warn("取消任务失败", "task_id", task.TaskID, "error", err)
			}
			continue
		}
		if err := s.runTask(ctx, task); err != nil {
			s.logger.Warn("执行任务失败", "task_id", task.TaskID, "error", err)
		}
	}
	return nil
}

// maxStepsPerRun 限制单次 runTask 内推进的 step 数,预算之外的安全兜底。
const maxStepsPerRun = 100

// stepOutcome 表示一个 step 的推进结果。
type stepOutcome int

const (
	stepAdvanced stepOutcome = iota // step 完成,继续下一步
	stepFinal                       // 计划完成,任务成功
	stepWaiting                     // 等待人工审批,workflow 暂停
	stepHalted                      // 已落终态(失败/超限/取消)
)

// runTask 执行单个任务的本地 workflow:按计划逐 step 推进,在 step 边界检查取消、预算和循环。
func (s *Service) runTask(ctx context.Context, task db.AgentTask) error {
	locked, ok, err := s.lockQueuedTask(ctx, task)
	if err != nil || !ok {
		return err
	}
	task = locked
	_ = s.appendEvent(ctx, task, domainevent.TypeTaskStarted, map[string]any{"workflow_id": task.WorkflowID}, nil)
	for i := 0; i < maxStepsPerRun; i++ {
		if s.cancelStore.IsCanceled(ctx, task.TaskID) {
			return s.finishCanceledTask(ctx, task)
		}
		_, budgetSpan := tracing.StartSpan(ctx, "budget.check", task.TaskID, task.TenantID)
		budgetErr := s.checkBudget(task)
		budgetSpan.End()
		if budgetErr != nil {
			return s.stopTaskByLimit(ctx, task, "BUDGET_EXCEEDED", budgetErr.Error())
		}
		state, err := s.queries.GetAgentState(ctx, db.GetAgentStateParams{
			TenantID: task.TenantID,
			TaskID:   task.TaskID,
		})
		if err != nil {
			return s.failTask(ctx, task, "STATE_NOT_FOUND", "任务状态不存在")
		}
		outcome, updatedTask, err := s.runStep(ctx, task, state)
		if err != nil {
			return err
		}
		task = updatedTask
		if outcome != stepAdvanced {
			return nil
		}
	}
	return s.stopTaskByLimit(ctx, task, "STEP_LIMIT_EXCEEDED", fmt.Sprintf("单次执行超过 %d 个 step 安全上限", maxStepsPerRun))
}

// runStep 推进一个 step:复用或调用 LLM、循环检测、工具调用和状态提交。
func (s *Service) runStep(ctx context.Context, task db.AgentTask, state db.AgentState) (stepOutcome, db.AgentTask, error) {
	ctx, span := tracing.StartSpan(ctx, "agent.step", task.TaskID, task.TenantID, attribute.Int("stableagent.step", int(state.CurrentStep)))
	defer span.End()
	_ = s.appendEvent(ctx, task, domainevent.TypeStepStarted, map[string]any{"step": state.CurrentStep}, nil)
	if _, err := s.createCheckpoint(ctx, task, state, "BEFORE_LLM", nil); err != nil {
		return stepHalted, task, s.failTask(ctx, task, "CHECKPOINT_FAILED", err.Error())
	}

	llmResp, reused, err := s.loadReusableLLMResponse(ctx, task, state)
	if err != nil {
		// 复用检查失败不阻断任务,退回重新调用 LLM。
		s.logger.Warn("读取可复用 LLM 输出失败", "task_id", task.TaskID, "error", err)
		reused = false
	}
	if reused {
		_ = s.appendEvent(ctx, task, domainevent.TypeLLMCallCompleted,
			map[string]any{"step": state.CurrentStep, "reused_from_checkpoint": true},
			map[string]any{"content": llmResp.Content})
	} else {
		_ = s.appendEvent(ctx, task, domainevent.TypeLLMCallStarted, map[string]any{"step": state.CurrentStep}, nil)
		llmResp, err = s.callLLM(ctx, task, state)
		if err != nil {
			return stepHalted, task, s.failTask(ctx, task, "LLM_CALL_FAILED", err.Error())
		}
		_ = s.appendEvent(ctx, task, domainevent.TypeLLMCallCompleted, map[string]any{"model": llmResp.Model}, map[string]any{"content": llmResp.Content, "usage": llmResp.Usage})
		if _, err := s.createCheckpoint(ctx, task, state, "AFTER_LLM", &llmResp); err != nil {
			return stepHalted, task, s.failTask(ctx, task, "CHECKPOINT_FAILED", err.Error())
		}
	}

	fingerprints, err := decodeFingerprints(state.LoopFingerprints)
	if err != nil {
		return stepHalted, task, s.failTask(ctx, task, "STATE_CORRUPTED", "loop_fingerprints 解析失败: "+err.Error())
	}

	toolCalled := false
	if llmResp.ToolCall != nil && !llmResp.IsFinal {
		arguments, err := domaintool.NormalizeArguments(llmResp.ToolCall.Arguments)
		if err != nil {
			return stepHalted, task, s.failTask(ctx, task, "TOOL_ARGUMENTS_INVALID", err.Error())
		}
		_, loopSpan := tracing.StartSpan(ctx, "loop.detect", task.TaskID, task.TenantID)
		fingerprint := loopdetect.BuildFingerprint(llmResp.ToolCall.ToolName, domaintool.HashArguments(arguments), llmResp.Content)
		detection := loopdetect.Detect(fingerprints, fingerprint, loopdetect.DefaultThreshold)
		loopSpan.SetAttributes(attribute.Bool("stableagent.loop_detected", detection.Detected), attribute.Int("stableagent.loop_repeats", detection.Repeats))
		loopSpan.End()
		if detection.Detected {
			return stepHalted, task, s.stopTaskByLimit(ctx, task, "LOOP_DETECTED",
				fmt.Sprintf("step 指纹连续重复 %d 次(阈值 %d),判定为循环", detection.Repeats, detection.Threshold))
		}
		fingerprints = loopdetect.Append(fingerprints, fingerprint)

		toolResult, err := s.callTool(ctx, task, state, *llmResp.ToolCall)
		if err != nil {
			return stepHalted, task, s.failTask(ctx, task, "TOOL_CALL_FAILED", err.Error())
		}
		if toolResult.Status == string(db.ToolCallStatusWAITINGAPPROVAL) {
			return stepWaiting, task, nil
		}
		if toolResult.Status == string(db.ToolCallStatusFAILED) {
			return stepHalted, task, s.failTask(ctx, task, firstPtr(toolResult.ErrorCode, "TOOL_FAILED"), firstPtr(toolResult.ErrorMessage, "工具调用失败"))
		}
		toolCalled = true
	}

	updatedTask, err := s.commitStep(ctx, task, state, llmResp, fingerprints, toolCalled)
	if err != nil {
		return stepHalted, task, err
	}
	if llmResp.IsFinal {
		if _, err := s.queries.UpdateAgentTaskStatus(ctx, db.UpdateAgentTaskStatusParams{
			TenantID: task.TenantID,
			TaskID:   task.TaskID,
			Status:   db.TaskStatusSUCCEEDED,
		}); err != nil {
			return stepHalted, updatedTask, mapWriteError(err)
		}
		_ = s.appendEvent(ctx, task, domainevent.TypeTaskCompleted, nil, map[string]any{"status": db.TaskStatusSUCCEEDED})
		metrics.TasksFinishedTotal.WithLabelValues(string(db.TaskStatusSUCCEEDED)).Inc()
		return stepFinal, updatedTask, nil
	}
	return stepAdvanced, updatedTask, nil
}

// loadReusableLLMResponse 查询当前 step 是否已有 AFTER_LLM checkpoint,崩溃恢复时复用已保存输出。
func (s *Service) loadReusableLLMResponse(ctx context.Context, task db.AgentTask, state db.AgentState) (runtimeplan.ChatResponse, bool, error) {
	checkpoint, err := s.queries.GetLatestCheckpointByReason(ctx, db.GetLatestCheckpointByReasonParams{
		TenantID: task.TenantID,
		TaskID:   task.TaskID,
		Reason:   "AFTER_LLM",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return runtimeplan.ChatResponse{}, false, nil
	}
	if err != nil {
		return runtimeplan.ChatResponse{}, false, err
	}
	var snapshot struct {
		CurrentStep int32                     `json:"current_step"`
		LLMResponse *runtimeplan.ChatResponse `json:"llm_response"`
	}
	if err := json.Unmarshal(checkpoint.StateSnapshot, &snapshot); err != nil {
		return runtimeplan.ChatResponse{}, false, err
	}
	if snapshot.CurrentStep != state.CurrentStep || snapshot.LLMResponse == nil {
		return runtimeplan.ChatResponse{}, false, nil
	}
	return *snapshot.LLMResponse, true, nil
}

// lockQueuedTask 将 QUEUED 任务原子切换为 RUNNING。
func (s *Service) lockQueuedTask(ctx context.Context, task db.AgentTask) (db.AgentTask, bool, error) {
	row := s.pool.QueryRow(ctx, `
update agent_tasks
set status = 'RUNNING',
    updated_at = now()
where tenant_id = $1
  and task_id = $2
  and status = 'QUEUED'
returning task_id, tenant_id, user_id, goal, status, budget, budget_usage, workflow_id, trace_id,
          last_error_code, last_error_message, created_at, updated_at`, task.TenantID, task.TaskID)
	var locked db.AgentTask
	err := row.Scan(
		&locked.TaskID,
		&locked.TenantID,
		&locked.UserID,
		&locked.Goal,
		&locked.Status,
		&locked.Budget,
		&locked.BudgetUsage,
		&locked.WorkflowID,
		&locked.TraceID,
		&locked.LastErrorCode,
		&locked.LastErrorMessage,
		&locked.CreatedAt,
		&locked.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AgentTask{}, false, nil
	}
	return locked, err == nil, err
}

// callLLM 调用 LLM Gateway 获取下一步计划。
func (s *Service) callLLM(ctx context.Context, task db.AgentTask, state db.AgentState) (runtimeplan.ChatResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "llm.call", task.TaskID, task.TenantID, attribute.Int("stableagent.step", int(state.CurrentStep)))
	defer span.End()
	req := runtimeplan.ChatRequest{
		TaskID:      task.TaskID,
		TenantID:    task.TenantID,
		Goal:        task.Goal,
		CurrentStep: state.CurrentStep,
		Constraints: state.Constraints,
	}
	var envelope struct {
		Data runtimeplan.ChatResponse `json:"data"`
	}
	startedAt := time.Now()
	if err := s.postJSON(ctx, s.llmURL+"/llm/v1/chat", req, &envelope); err != nil {
		metrics.LLMCallDuration.WithLabelValues("error").Observe(time.Since(startedAt).Seconds())
		return runtimeplan.ChatResponse{}, err
	}
	metrics.LLMCallDuration.WithLabelValues("ok").Observe(time.Since(startedAt).Seconds())
	metrics.LLMTokensTotal.Add(float64(envelope.Data.Usage.TotalTokens))
	return envelope.Data, nil
}

// callTool 调用 Tool Gateway 并传递幂等键。
func (s *Service) callTool(ctx context.Context, task db.AgentTask, state db.AgentState, intent runtimeplan.ToolIntent) (toolusecase.CallResult, error) {
	ctx, span := tracing.StartSpan(ctx, "tool.call", task.TaskID, task.TenantID, attribute.String("stableagent.tool_name", intent.ToolName))
	defer span.End()
	arguments, err := domaintool.NormalizeArguments(intent.Arguments)
	if err != nil {
		return toolusecase.CallResult{}, err
	}
	argumentsHash := domaintool.HashArguments(arguments)
	req := toolusecase.CallInput{
		TenantID:       task.TenantID,
		UserID:         task.UserID,
		TaskID:         task.TaskID,
		ToolName:       intent.ToolName,
		Arguments:      arguments,
		IdempotencyKey: domaintool.BuildIdempotencyKey(task.TaskID, state.CurrentStep, intent.ToolName, argumentsHash),
		TraceID:        task.TraceID,
	}
	var envelope struct {
		Data toolusecase.CallResult `json:"data"`
	}
	startedAt := time.Now()
	if err := s.postJSON(ctx, s.toolURL+"/tools/v1/calls", req, &envelope); err != nil {
		metrics.ToolCallDuration.WithLabelValues(intent.ToolName, "error").Observe(time.Since(startedAt).Seconds())
		return toolusecase.CallResult{}, err
	}
	metrics.ToolCallDuration.WithLabelValues(intent.ToolName, strings.ToLower(envelope.Data.Status)).Observe(time.Since(startedAt).Seconds())
	return envelope.Data, nil
}

// commitStep 提交一个已完成 step:更新 AgentState(乐观锁)、预算用量和 AFTER_TOOL checkpoint。
func (s *Service) commitStep(ctx context.Context, task db.AgentTask, state db.AgentState, llmResp runtimeplan.ChatResponse, fingerprints []string, toolCalled bool) (db.AgentTask, error) {
	usage, err := incrementBudgetUsage(task.BudgetUsage, llmResp.Usage, toolCalled)
	if err != nil {
		return task, s.failTask(ctx, task, "BUDGET_USAGE_INVALID", err.Error())
	}
	currentPlan, _ := json.Marshal(map[string]any{
		"model":          llmResp.Model,
		"prompt_version": llmResp.PromptVersion,
		"content":        llmResp.Content,
	})
	fingerprintsJSON, err := json.Marshal(fingerprints)
	if err != nil {
		return task, s.failTask(ctx, task, "STATE_CORRUPTED", err.Error())
	}
	updatedState, err := s.queries.UpdateAgentStateOptimistic(ctx, db.UpdateAgentStateOptimisticParams{
		CurrentPlan:      currentPlan,
		CurrentStep:      state.CurrentStep + 1,
		MemorySummary:    stringPtr(fmt.Sprintf("已完成第 %d 步: %s", state.CurrentStep+1, llmResp.Content)),
		Constraints:      state.Constraints,
		Artifacts:        state.Artifacts,
		LoopFingerprints: fingerprintsJSON,
		TenantID:         task.TenantID,
		TaskID:           task.TaskID,
		Version:          state.Version,
	})
	if err != nil {
		return task, s.failTask(ctx, task, "STATE_CONFLICT", err.Error())
	}
	updatedTask, err := s.queries.UpdateAgentTaskBudgetUsage(ctx, db.UpdateAgentTaskBudgetUsageParams{
		BudgetUsage: usage,
		TenantID:    task.TenantID,
		TaskID:      task.TaskID,
	})
	if err != nil {
		return task, s.failTask(ctx, task, "BUDGET_UPDATE_FAILED", err.Error())
	}
	if _, err := s.createCheckpoint(ctx, task, updatedState, "AFTER_TOOL", nil); err != nil {
		return updatedTask, s.failTask(ctx, task, "CHECKPOINT_FAILED", err.Error())
	}
	_ = s.appendEvent(ctx, task, domainevent.TypeStepCompleted, map[string]any{"step": state.CurrentStep}, map[string]any{"next_step": updatedState.CurrentStep, "tool_called": toolCalled})
	metrics.StepsTotal.Inc()
	return updatedTask, nil
}

// stopTaskByLimit 将任务落到 STOPPED_BY_LIMIT,预算超限和循环检测共用,可通过 resume 恢复。
func (s *Service) stopTaskByLimit(ctx context.Context, task db.AgentTask, code string, message string) error {
	if _, err := s.queries.MarkAgentTaskStopped(ctx, db.MarkAgentTaskStoppedParams{
		LastErrorCode:    &code,
		LastErrorMessage: &message,
		TenantID:         task.TenantID,
		TaskID:           task.TaskID,
	}); err != nil {
		return mapWriteError(err)
	}
	_ = s.appendEvent(ctx, task, domainevent.TypeTaskStoppedByLimit, map[string]any{"error_code": code}, map[string]any{"error_message": message, "status": db.TaskStatusSTOPPEDBYLIMIT})
	metrics.TasksFinishedTotal.WithLabelValues(string(db.TaskStatusSTOPPEDBYLIMIT)).Inc()
	return nil
}

// decodeFingerprints 解析 agent_states.loop_fingerprints 历史。
func decodeFingerprints(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var fingerprints []string
	if err := json.Unmarshal(raw, &fingerprints); err != nil {
		return nil, err
	}
	return fingerprints, nil
}

// finishCanceledTask 将任务安全落到 CANCELED 终态。
func (s *Service) finishCanceledTask(ctx context.Context, task db.AgentTask) error {
	if _, err := s.queries.UpdateAgentTaskStatus(ctx, db.UpdateAgentTaskStatusParams{
		TenantID: task.TenantID,
		TaskID:   task.TaskID,
		Status:   db.TaskStatusCANCELED,
	}); err != nil {
		return mapWriteError(err)
	}
	if s.cancelStore != nil {
		_ = s.cancelStore.Clear(ctx, task.TaskID)
	}
	_ = s.appendEvent(ctx, task, domainevent.TypeTaskCancelled, map[string]any{"previous_status": task.Status}, map[string]any{"status": db.TaskStatusCANCELED})
	metrics.TasksFinishedTotal.WithLabelValues(string(db.TaskStatusCANCELED)).Inc()
	return nil
}

// failTask 标记任务失败并写入失败事件。
func (s *Service) failTask(ctx context.Context, task db.AgentTask, code string, message string) error {
	if _, err := s.queries.MarkAgentTaskFailed(ctx, db.MarkAgentTaskFailedParams{
		LastErrorCode:    &code,
		LastErrorMessage: &message,
		TenantID:         task.TenantID,
		TaskID:           task.TaskID,
	}); err != nil {
		return mapWriteError(err)
	}
	_ = s.appendEvent(ctx, task, domainevent.TypeTaskFailed, map[string]any{"error_code": code}, map[string]any{"error_message": message})
	metrics.TasksFinishedTotal.WithLabelValues(string(db.TaskStatusFAILED)).Inc()
	return nil
}

// createCheckpoint 保存当前 AgentState 快照;AFTER_LLM checkpoint 额外保存 LLM 输出供崩溃恢复复用。
func (s *Service) createCheckpoint(ctx context.Context, task db.AgentTask, state db.AgentState, reason string, llmResp *runtimeplan.ChatResponse) (db.Checkpoint, error) {
	ctx, span := tracing.StartSpan(ctx, "checkpoint.create", task.TaskID, task.TenantID, attribute.String("stableagent.checkpoint_reason", reason))
	defer span.End()
	checkpointID, err := ids.New("ckpt")
	if err != nil {
		return db.Checkpoint{}, err
	}
	offset, err := s.latestEventOffset(ctx, task.TenantID, task.TaskID)
	if err != nil {
		return db.Checkpoint{}, err
	}
	snapshotFields := map[string]any{
		"schema_version":    1,
		"current_plan":      json.RawMessage(state.CurrentPlan),
		"current_step":      state.CurrentStep,
		"memory_summary":    state.MemorySummary,
		"constraints":       json.RawMessage(state.Constraints),
		"artifacts":         json.RawMessage(state.Artifacts),
		"loop_fingerprints": json.RawMessage(state.LoopFingerprints),
		"version":           state.Version,
	}
	if llmResp != nil {
		snapshotFields["llm_response"] = llmResp
	}
	snapshot, err := json.Marshal(snapshotFields)
	if err != nil {
		return db.Checkpoint{}, err
	}
	checkpoint, err := s.queries.CreateCheckpoint(ctx, db.CreateCheckpointParams{
		CheckpointID:  checkpointID,
		TenantID:      task.TenantID,
		TaskID:        task.TaskID,
		StateVersion:  state.Version,
		EventOffset:   offset,
		StateSnapshot: snapshot,
		Reason:        reason,
		TraceID:       task.TraceID,
	})
	if err != nil {
		return db.Checkpoint{}, mapWriteError(err)
	}
	_ = s.appendEvent(ctx, task, domainevent.TypeCheckpointCreated, map[string]any{"reason": reason}, map[string]any{"checkpoint_id": checkpoint.CheckpointID})
	// 设计方案 §6.6:每个任务只保留最近 checkpointRetention 个 checkpoint 热数据。
	if err := s.queries.PruneCheckpoints(ctx, db.PruneCheckpointsParams{
		TenantID: task.TenantID,
		TaskID:   task.TaskID,
		KeepRows: checkpointRetention,
	}); err != nil {
		s.logger.Warn("裁剪历史 checkpoint 失败", "task_id", task.TaskID, "error", err)
	}
	return checkpoint, nil
}

// checkpointRetention 定义每个任务保留的最近 checkpoint 数量。
const checkpointRetention = 20

// latestEventOffset 查询任务当前最大的 event_id。
func (s *Service) latestEventOffset(ctx context.Context, tenantID string, taskID string) (int64, error) {
	var offset int64
	err := s.pool.QueryRow(ctx, `select coalesce(max(event_id), 0) from agent_events where tenant_id = $1 and task_id = $2`, tenantID, taskID).Scan(&offset)
	return offset, err
}

// appendEvent 追加 Worker 执行事件。
func (s *Service) appendEvent(ctx context.Context, task db.AgentTask, eventType domainevent.Type, eventInput any, eventOutput any) error {
	if s.events == nil {
		return nil
	}
	input, err := marshalJSON(eventInput)
	if err != nil {
		return err
	}
	output, err := marshalJSON(eventOutput)
	if err != nil {
		return err
	}
	metadata, err := domainevent.MarshalMetadata(domainevent.Metadata{
		UserID:     task.UserID,
		Source:     "runtime-worker",
		WorkflowID: valueOrEmpty(task.WorkflowID),
	})
	if err != nil {
		return err
	}
	_, err = s.events.Append(ctx, eventapi.AppendEventInput{
		TenantID: task.TenantID,
		TaskID:   task.TaskID,
		Type:     string(eventType),
		Input:    input,
		Output:   output,
		Metadata: metadata,
		TraceID:  task.TraceID,
	})
	return err
}

// checkBudget 校验任务预算是否允许继续执行。
func (s *Service) checkBudget(task db.AgentTask) error {
	var budget domaintask.Budget
	if err := json.Unmarshal(task.Budget, &budget); err != nil {
		return err
	}
	var usage domaintask.BudgetUsage
	if err := json.Unmarshal(task.BudgetUsage, &usage); err != nil {
		return err
	}
	if usage.UsedSteps >= budget.MaxSteps {
		return fmt.Errorf("step 已达到预算上限 %d", budget.MaxSteps)
	}
	if usage.UsedTokens >= budget.MaxTokens {
		return fmt.Errorf("token 已达到预算上限 %d", budget.MaxTokens)
	}
	if usage.UsedToolCalls >= budget.MaxToolCalls {
		return fmt.Errorf("tool call 已达到预算上限 %d", budget.MaxToolCalls)
	}
	if usage.UsedCostUSD >= budget.MaxCostUSD {
		return fmt.Errorf("成本已达到预算上限 %.4f", budget.MaxCostUSD)
	}
	return nil
}

// incrementBudgetUsage 根据 LLM 返回用量更新预算使用量,只有实际发生工具调用的 step 才累计 tool call 用量。
func incrementBudgetUsage(raw json.RawMessage, usage runtimeplan.TokenUsage, toolCalled bool) (json.RawMessage, error) {
	var current domaintask.BudgetUsage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &current); err != nil {
			return nil, err
		}
	}
	current.UsedSteps++
	current.UsedTokens += usage.TotalTokens
	if toolCalled {
		current.UsedToolCalls++
	}
	current.UsedCostUSD += usage.CostUSD
	return json.Marshal(current)
}

// postJSON 发送 JSON POST 请求并解析统一响应。
func (s *Service) postJSON(ctx context.Context, url string, payload any, dst any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	tracing.Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s 返回 %d: %s", url, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, dst)
}

// marshalJSON 将事件字段编码为 JSON。
func marshalJSON(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage(`null`), nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		return raw, nil
	}
	data, err := json.Marshal(value)
	return data, err
}

// mapWriteError 包装 Worker 数据库写入错误。
func mapWriteError(err error) error {
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库写入失败", err)
}

// valueOrEmpty 返回字符串指针的值或空字符串。
func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// firstPtr 返回字符串指针的值或默认值。
func firstPtr(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return strings.TrimSpace(*value)
}

// stringPtr 返回字符串指针。
func stringPtr(value string) *string {
	return &value
}
