package taskapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	domainevent "stableagent/internal/domain/event"
	domaintask "stableagent/internal/domain/task"
	"stableagent/internal/infra/postgres/db"
	"stableagent/internal/observability/tracing"
	"stableagent/internal/security/authn"
	"stableagent/internal/security/authz"
	"stableagent/internal/usecase/auditlog"
	apperrors "stableagent/pkg/errors"
	"stableagent/pkg/ids"
)

const (
	defaultPageSize     = 20
	maxPageSize         = 100
	outboxStartWorkflow = "START_WORKFLOW"
)

type Service struct {
	pool          *pgxpool.Pool
	queries       *db.Queries
	eventNotifier persistedEventNotifier
	cancelStore   cancelFlagStore
	pauseStore    pauseFlagStore
	auditLogger   *auditlog.Service
}

type persistedEventNotifier interface {
	NotifyPersisted(ctx context.Context, event domainevent.AgentEvent)
}

type cancelFlagStore interface {
	Set(ctx context.Context, taskID string) error
}

// pauseFlagStore 抽象暂停标记存储,Worker 在 step 边界读取该标记安全停下。
type pauseFlagStore interface {
	Set(ctx context.Context, taskID string) error
	Clear(ctx context.Context, taskID string) error
}

type CreateTaskInput struct {
	TenantID        string
	UserID          string
	UserRole        string
	RequestID       string
	ClientRequestID string
	Goal            string
	Budget          domaintask.Budget
	Constraints     json.RawMessage
	// IP 和 UserAgent 仅用于审计日志记录,不参与业务逻辑。
	IP        string
	UserAgent string
}

type CreatedTask struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type GetTaskInput struct {
	TenantID string
	UserID   string
	UserRole string
	TaskID   string
}

type TaskDetail struct {
	TaskID           string          `json:"task_id"`
	Goal             string          `json:"goal"`
	Status           string          `json:"status"`
	CurrentStep      int32           `json:"current_step"`
	Budget           json.RawMessage `json:"budget"`
	BudgetUsage      json.RawMessage `json:"budget_usage"`
	TraceID          *string         `json:"trace_id,omitempty"`
	LastErrorCode    *string         `json:"last_error_code,omitempty"`
	LastErrorMessage *string         `json:"last_error_message,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type ListTasksInput struct {
	TenantID        string
	UserID          string
	UserRole        string
	Status          string
	Goal            string
	FilterUserID    string
	FilterTenantID  string
	CreatedFrom     string
	CreatedTo       string
	WaitingApproval bool
	StoppedByLimit  bool
	Page            int
	PageSize        int
	Sort            string
}

type TaskSummary struct {
	TaskID      string          `json:"task_id"`
	TenantID    string          `json:"tenant_id,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	Goal        string          `json:"goal"`
	Status      string          `json:"status"`
	Budget      json.RawMessage `json:"budget"`
	BudgetUsage json.RawMessage `json:"budget_usage"`
	TraceID     *string         `json:"trace_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type TaskPage struct {
	Items    []TaskSummary `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	HasMore  bool          `json:"has_more"`
	NextPage *int          `json:"next_page,omitempty"`
	Sort     string        `json:"sort"`
	Status   string        `json:"status,omitempty"`
}

// NewService 创建任务 API 用例服务。
func NewService(pool *pgxpool.Pool) *Service {
	return NewServiceWithEventNotifier(pool, nil)
}

// NewServiceWithEventNotifier 创建带事件推送通知能力的任务 API 用例服务。
func NewServiceWithEventNotifier(pool *pgxpool.Pool, notifier persistedEventNotifier) *Service {
	return NewServiceWithEventNotifierAndCancelStore(pool, notifier, nil)
}

// NewServiceWithEventNotifierAndCancelStore 创建带事件推送和取消标记能力的任务 API 用例服务。
func NewServiceWithEventNotifierAndCancelStore(pool *pgxpool.Pool, notifier persistedEventNotifier, cancelStore cancelFlagStore) *Service {
	return NewServiceWithDeps(pool, notifier, cancelStore, nil, nil)
}

// NewServiceWithDeps 创建带完整依赖(事件推送、取消标记、暂停标记、审计日志)的任务 API 用例服务。
func NewServiceWithDeps(pool *pgxpool.Pool, notifier persistedEventNotifier, cancelStore cancelFlagStore, pauseStore pauseFlagStore, auditLogger *auditlog.Service) *Service {
	return &Service{
		pool:          pool,
		queries:       db.New(pool),
		eventNotifier: notifier,
		cancelStore:   cancelStore,
		pauseStore:    pauseStore,
		auditLogger:   auditLogger,
	}
}

// CreateTask 创建 Agent 任务，并在同一事务内写入初始状态、审计事件和 outbox 消息。
func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (CreatedTask, error) {
	ctx, span := tracing.StartSpan(ctx, "task.create", "", input.TenantID)
	defer span.End()
	goal, err := domaintask.NormalizeGoal(input.Goal)
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInvalidArg, err.Error(), err)
	}
	if err := domaintask.ValidateBudget(input.Budget); err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInvalidArg, err.Error(), err)
	}
	clientRequestID := strings.TrimSpace(input.ClientRequestID)
	if clientRequestID != "" {
		task, err := s.getTaskByClientRequestID(ctx, input.TenantID, input.UserID, clientRequestID)
		if err == nil {
			return CreatedTask{
				TaskID:    task.TaskID,
				Status:    domaintask.StatusForAPI(string(task.Status)),
				CreatedAt: pgTime(task.CreatedAt),
			}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return CreatedTask{}, mapPostgresReadError(err)
		}
	}
	constraints, err := domaintask.NormalizeJSONObject(input.Constraints)
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInvalidArg, "constraints "+err.Error(), err)
	}

	taskID, err := ids.New("task")
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "生成 task_id 失败", err)
	}
	traceID, err := ids.New("trace")
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "生成 trace_id 失败", err)
	}
	spanID, err := ids.New("span")
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "生成 span_id 失败", err)
	}
	budgetJSON, err := json.Marshal(input.Budget)
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化 budget 失败", err)
	}
	budgetUsageJSON, err := json.Marshal(domaintask.BudgetUsage{})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化 budget_usage 失败", err)
	}

	// 任务、状态、事件和 outbox 必须原子提交，否则 worker 可能看到不完整任务或漏启动 workflow。
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeUnavailable, "创建任务事务失败", err)
	}
	defer func() {
		// Commit 成功后 Rollback 会返回已提交错误，这里忽略即可；失败路径会自动回滚未提交写入。
		_ = tx.Rollback(ctx)
	}()

	queries := db.New(tx)
	task, err := queries.CreateAgentTask(ctx, db.CreateAgentTaskParams{
		TaskID:      taskID,
		TenantID:    input.TenantID,
		UserID:      input.UserID,
		Goal:        goal,
		Status:      db.TaskStatusQUEUED,
		Budget:      budgetJSON,
		BudgetUsage: budgetUsageJSON,
		TraceID:     &traceID,
	})
	if err != nil {
		return CreatedTask{}, mapPostgresWriteError(err)
	}

	if _, err := queries.CreateAgentState(ctx, db.CreateAgentStateParams{
		TenantID:         input.TenantID,
		TaskID:           taskID,
		CurrentPlan:      json.RawMessage(`{}`),
		CurrentStep:      0,
		Constraints:      constraints,
		Artifacts:        json.RawMessage(`[]`),
		LoopFingerprints: json.RawMessage(`[]`),
	}); err != nil {
		return CreatedTask{}, mapPostgresWriteError(err)
	}

	eventInput, err := json.Marshal(map[string]any{
		"goal":        goal,
		"budget":      input.Budget,
		"constraints": json.RawMessage(constraints),
	})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化任务事件输入失败", err)
	}
	// 初始事件使用统一 metadata 结构，方便 timeline、审计和日志按同一字段消费。
	eventMetadata, err := domainevent.MarshalMetadata(domainevent.Metadata{
		UserID:    input.UserID,
		RequestID: input.RequestID,
		Source:    "api-service",
	})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化任务事件元数据失败", err)
	}
	createdEvent, err := queries.CreateAgentEvent(ctx, db.CreateAgentEventParams{
		TenantID: input.TenantID,
		TaskID:   taskID,
		Type:     domainevent.MustTypeForStorage(domainevent.TypeTaskCreated),
		Input:    eventInput,
		Output:   json.RawMessage(`null`),
		Metadata: eventMetadata,
		TraceID:  &traceID,
		SpanID:   &spanID,
	})
	if err != nil {
		return CreatedTask{}, mapPostgresWriteError(err)
	}

	outboxPayload, err := json.Marshal(map[string]any{
		"task_id":    taskID,
		"tenant_id":  input.TenantID,
		"user_id":    input.UserID,
		"trace_id":   traceID,
		"span_id":    spanID,
		"request_id": input.RequestID,
	})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化 outbox payload 失败", err)
	}
	// outbox 与任务同事务写入，保证创建成功的任务最终一定能被 worker 拉起。
	if _, err := queries.CreateTaskOutboxMessage(ctx, db.CreateTaskOutboxMessageParams{
		TenantID:      input.TenantID,
		AggregateType: "agent_task",
		AggregateID:   taskID,
		EventType:     outboxStartWorkflow,
		Payload:       outboxPayload,
		Status:        db.OutboxStatusPENDING,
		NextRetryAt:   pgtype.Timestamptz{},
	}); err != nil {
		return CreatedTask{}, mapPostgresWriteError(err)
	}
	if clientRequestID != "" {
		inserted, err := s.insertTaskIdempotencyKey(ctx, tx, input.TenantID, input.UserID, clientRequestID, taskID)
		if err != nil {
			return CreatedTask{}, mapPostgresWriteError(err)
		}
		if !inserted {
			existing, readErr := s.getTaskByClientRequestID(ctx, input.TenantID, input.UserID, clientRequestID)
			if readErr != nil {
				return CreatedTask{}, apperrors.Wrap(apperrors.CodeConflict, "client_request_id 已存在但任务暂不可读", readErr)
			}
			return CreatedTask{
				TaskID:    existing.TaskID,
				Status:    domaintask.StatusForAPI(string(existing.Status)),
				CreatedAt: pgTime(existing.CreatedAt),
			}, nil
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeUnavailable, "提交任务创建事务失败", err)
	}
	if s.eventNotifier != nil {
		// 事件必须在事务提交后推送，避免客户端收到数据库尚不可查询的 event_id。
		s.eventNotifier.NotifyPersisted(ctx, eventFromCreatedRow(createdEvent))
	}
	s.audit(ctx, auditlog.LogInput{
		ActorID:      input.UserID,
		TenantID:     input.TenantID,
		Action:       "task.create",
		ResourceType: "task",
		ResourceID:   task.TaskID,
		After:        map[string]any{"goal": goal, "budget": input.Budget, "status": string(task.Status)},
		IP:           input.IP,
		UserAgent:    input.UserAgent,
	})

	return CreatedTask{
		TaskID:    task.TaskID,
		Status:    domaintask.StatusForAPI(string(task.Status)),
		CreatedAt: pgTime(task.CreatedAt),
	}, nil
}

// audit 是审计日志写入的便捷包装,未配置 auditLogger 时静默跳过。
func (s *Service) audit(ctx context.Context, input auditlog.LogInput) {
	if s.auditLogger == nil {
		return
	}
	_ = s.auditLogger.Log(ctx, input)
}

// getTaskByClientRequestID 根据客户端幂等键查询已创建任务。
func (s *Service) getTaskByClientRequestID(ctx context.Context, tenantID string, userID string, clientRequestID string) (db.AgentTask, error) {
	row := s.pool.QueryRow(ctx, `
select agent_tasks.task_id, agent_tasks.tenant_id, agent_tasks.user_id, agent_tasks.goal, agent_tasks.status,
       agent_tasks.budget, agent_tasks.budget_usage, agent_tasks.workflow_id, agent_tasks.trace_id,
       agent_tasks.last_error_code, agent_tasks.last_error_message, agent_tasks.created_at, agent_tasks.updated_at
from task_idempotency_keys
join agent_tasks
  on agent_tasks.tenant_id = task_idempotency_keys.tenant_id
 and agent_tasks.task_id = task_idempotency_keys.task_id
where task_idempotency_keys.tenant_id = $1
  and task_idempotency_keys.user_id = $2
  and task_idempotency_keys.client_request_id = $3
limit 1`, tenantID, userID, clientRequestID)
	var task db.AgentTask
	err := row.Scan(
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
	)
	return task, err
}

// insertTaskIdempotencyKey 在事务内保存客户端幂等键。
func (s *Service) insertTaskIdempotencyKey(ctx context.Context, tx pgx.Tx, tenantID string, userID string, clientRequestID string, taskID string) (bool, error) {
	tag, err := tx.Exec(ctx, `
insert into task_idempotency_keys (tenant_id, user_id, client_request_id, task_id)
values ($1, $2, $3, $4)
on conflict (tenant_id, user_id, client_request_id) do nothing`, tenantID, userID, clientRequestID, taskID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// eventFromCreatedRow 将创建任务事务中的事件记录转换为推送事件。
func eventFromCreatedRow(row db.AgentEvent) domainevent.AgentEvent {
	return domainevent.AgentEvent{
		EventID:   row.EventID,
		TaskID:    row.TaskID,
		TenantID:  row.TenantID,
		Type:      row.Type,
		Input:     domainevent.NormalizeJSONValue(row.Input),
		Output:    domainevent.NormalizeJSONValue(row.Output),
		Metadata:  domainevent.NormalizeMetadata(row.Metadata),
		TraceID:   row.TraceID,
		SpanID:    row.SpanID,
		CreatedAt: pgTime(row.CreatedAt),
	}
}

// GetTask 查询任务详情，并按租户和用户权限过滤访问。
func (s *Service) GetTask(ctx context.Context, input GetTaskInput) (TaskDetail, error) {
	task, err := s.queries.GetAgentTask(ctx, db.GetAgentTaskParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
	})
	if err != nil {
		return TaskDetail{}, mapPostgresReadError(err)
	}
	if !authz.CanReadTask(authn.User{TenantID: input.TenantID, UserID: input.UserID, Role: input.UserRole}, task.UserID) {
		return TaskDetail{}, apperrors.New(apperrors.CodeForbidden, "没有访问该任务的权限")
	}

	var currentStep int32
	state, err := s.queries.GetAgentState(ctx, db.GetAgentStateParams{
		TenantID: input.TenantID,
		TaskID:   input.TaskID,
	})
	if err == nil {
		currentStep = state.CurrentStep
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return TaskDetail{}, mapPostgresReadError(err)
	}
	// 历史或异常任务可能暂时没有 state，详情仍返回任务主体，current_step 保持零值。

	return taskDetailFromDB(task, currentStep), nil
}

// ListTasks 查询任务列表，普通用户仅看自己的任务，管理员和所有者可看租户内任务。
func (s *Service) ListTasks(ctx context.Context, input ListTasksInput) (TaskPage, error) {
	page, pageSize, err := normalizePagination(input.Page, input.PageSize)
	if err != nil {
		return TaskPage{}, err
	}
	if input.Sort == "" {
		input.Sort = "created_at_desc"
	}
	if input.Sort != "created_at_desc" {
		return TaskPage{}, apperrors.New(apperrors.CodeInvalidArg, "sort 仅支持 created_at_desc")
	}

	var status domaintask.Status
	if input.Status != "" {
		parsed, ok := domaintask.ParseStatus(input.Status)
		if !ok {
			return TaskPage{}, apperrors.New(apperrors.CodeInvalidArg, "status 参数不合法")
		}
		status = parsed
	}

	tasks, err := s.listTasksFiltered(ctx, input, status, page, pageSize)
	if err != nil {
		return TaskPage{}, mapPostgresReadError(err)
	}

	hasMore := len(tasks) > pageSize
	if hasMore {
		tasks = tasks[:pageSize]
	}
	items := make([]TaskSummary, 0, len(tasks))
	for _, item := range tasks {
		items = append(items, taskSummaryFromDB(item))
	}

	var nextPage *int
	if hasMore {
		value := page + 1
		nextPage = &value
	}
	return TaskPage{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		HasMore:  hasMore,
		NextPage: nextPage,
		Sort:     input.Sort,
		Status:   string(status),
	}, nil
}

// taskDetailFromDB 将数据库任务记录转换为 API 详情响应。
func taskDetailFromDB(task db.AgentTask, currentStep int32) TaskDetail {
	return TaskDetail{
		TaskID:           task.TaskID,
		Goal:             task.Goal,
		Status:           domaintask.StatusForAPI(string(task.Status)),
		CurrentStep:      currentStep,
		Budget:           task.Budget,
		BudgetUsage:      task.BudgetUsage,
		TraceID:          task.TraceID,
		LastErrorCode:    task.LastErrorCode,
		LastErrorMessage: task.LastErrorMessage,
		CreatedAt:        pgTime(task.CreatedAt),
		UpdatedAt:        pgTime(task.UpdatedAt),
	}
}

// taskSummaryFromDB 将数据库任务记录转换为 API 列表摘要。
func taskSummaryFromDB(task db.AgentTask) TaskSummary {
	return TaskSummary{
		TaskID:      task.TaskID,
		TenantID:    task.TenantID,
		UserID:      task.UserID,
		Goal:        task.Goal,
		Status:      domaintask.StatusForAPI(string(task.Status)),
		Budget:      task.Budget,
		BudgetUsage: task.BudgetUsage,
		TraceID:     task.TraceID,
		CreatedAt:   pgTime(task.CreatedAt),
		UpdatedAt:   pgTime(task.UpdatedAt),
	}
}

// listTasksFiltered 使用设计契约中的扩展过滤条件查询任务列表。
func (s *Service) listTasksFiltered(ctx context.Context, input ListTasksInput, status domaintask.Status, page int, pageSize int) ([]db.AgentTask, error) {
	user := authn.User{TenantID: input.TenantID, UserID: input.UserID, Role: input.UserRole}
	conditions := []string{}
	args := []any{}
	tenantID, _ := authz.EffectiveTenantID(user, input.FilterTenantID)
	if tenantID != "" {
		conditions = append(conditions, fmt.Sprintf("tenant_id = %s", nextSQLArg(&args, tenantID)))
	}
	if !authz.CanListTenantTasks(user) {
		conditions = append(conditions, fmt.Sprintf("user_id = %s", nextSQLArg(&args, input.UserID)))
	} else if strings.TrimSpace(input.FilterUserID) != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = %s", nextSQLArg(&args, strings.TrimSpace(input.FilterUserID))))
	}
	effectiveStatus := status
	if input.WaitingApproval {
		effectiveStatus = domaintask.StatusWaitingApproval
	}
	if input.StoppedByLimit {
		effectiveStatus = domaintask.StatusStoppedByLimit
	}
	if effectiveStatus != "" {
		conditions = append(conditions, fmt.Sprintf("status = %s", nextSQLArg(&args, string(effectiveStatus))))
	}
	if goal := strings.TrimSpace(input.Goal); goal != "" {
		conditions = append(conditions, fmt.Sprintf("goal ilike %s", nextSQLArg(&args, "%"+goal+"%")))
	}
	if from, err := time.Parse(time.RFC3339, strings.TrimSpace(input.CreatedFrom)); err == nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= %s", nextSQLArg(&args, from.UTC())))
	}
	if to, err := time.Parse(time.RFC3339, strings.TrimSpace(input.CreatedTo)); err == nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= %s", nextSQLArg(&args, to.UTC())))
	}
	limit := nextSQLArg(&args, int32(pageSize+1))
	offset := nextSQLArg(&args, int32((page-1)*pageSize))
	query := fmt.Sprintf(`
select task_id, tenant_id, user_id, goal, status, budget, budget_usage, workflow_id, trace_id,
       last_error_code, last_error_message, created_at, updated_at
from agent_tasks
%s
order by created_at desc, task_id desc
limit %s offset %s`, whereClause(conditions), limit, offset)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []db.AgentTask{}
	for rows.Next() {
		var task db.AgentTask
		if err := rows.Scan(&task.TaskID, &task.TenantID, &task.UserID, &task.Goal, &task.Status, &task.Budget, &task.BudgetUsage, &task.WorkflowID, &task.TraceID, &task.LastErrorCode, &task.LastErrorMessage, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// nextSQLArg 追加 SQL 查询参数并返回 PostgreSQL 占位符。
func nextSQLArg(args *[]any, value any) string {
	*args = append(*args, value)
	return fmt.Sprintf("$%d", len(*args))
}

// whereClause 拼接动态 WHERE 子句。
func whereClause(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	return "where " + strings.Join(conditions, " and ")
}

// normalizePagination 规范化分页参数，并限制 offset 在 PostgreSQL int4 范围内。
func normalizePagination(page int, pageSize int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if page < 1 {
		return 0, 0, apperrors.New(apperrors.CodeInvalidArg, "page 必须大于等于 1")
	}
	if pageSize < 1 || pageSize > maxPageSize {
		return 0, 0, apperrors.New(apperrors.CodeInvalidArg, fmt.Sprintf("page_size 必须在 1 到 %d 之间", maxPageSize))
	}
	// sqlc 生成参数是 int32，提前拒绝超大 offset，避免整数截断导致错误分页。
	if (page-1)*pageSize > 2147483647 {
		return 0, 0, apperrors.New(apperrors.CodeInvalidArg, "分页 offset 超出支持范围")
	}
	return page, pageSize, nil
}

// mapPostgresReadError 将 PostgreSQL 读取错误转换为统一应用错误。
func mapPostgresReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.New(apperrors.CodeNotFound, "任务不存在")
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库读取失败", err)
}

// mapPostgresWriteError 将 PostgreSQL 写入错误转换为统一应用错误。
func mapPostgresWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			// 外键失败通常表示租户或用户不存在，不能暴露底层表结构细节。
			return apperrors.New(apperrors.CodeForbidden, "用户不属于租户或租户不存在")
		case "23505":
			return apperrors.New(apperrors.CodeConflict, "任务资源已存在")
		}
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库写入失败", err)
}

// pgTime 将 pgx 时间类型转换为 UTC time.Time，空值返回零时间。
func pgTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}
