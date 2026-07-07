package taskapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	domaintask "agent-runtime/internal/domain/task"
	"agent-runtime/internal/infra/postgres/db"
	"agent-runtime/internal/security/authn"
	"agent-runtime/internal/security/authz"
	apperrors "agent-runtime/pkg/errors"
	"agent-runtime/pkg/ids"
)

const (
	defaultPageSize     = 20
	maxPageSize         = 100
	taskEventCreated    = "TASK_CREATED"
	outboxStartWorkflow = "START_WORKFLOW"
)

type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

type CreateTaskInput struct {
	TenantID    string
	UserID      string
	UserRole    string
	RequestID   string
	Goal        string
	Budget      domaintask.Budget
	Constraints json.RawMessage
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
	TenantID string
	UserID   string
	UserRole string
	Status   string
	Page     int
	PageSize int
	Sort     string
}

type TaskSummary struct {
	TaskID      string          `json:"task_id"`
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

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:    pool,
		queries: db.New(pool),
	}
}

func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (CreatedTask, error) {
	goal, err := domaintask.NormalizeGoal(input.Goal)
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInvalidArg, err.Error(), err)
	}
	if err := domaintask.ValidateBudget(input.Budget); err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInvalidArg, err.Error(), err)
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
	budgetJSON, err := json.Marshal(input.Budget)
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化 budget 失败", err)
	}
	budgetUsageJSON, err := json.Marshal(domaintask.BudgetUsage{})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化 budget_usage 失败", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeUnavailable, "创建任务事务失败", err)
	}
	defer func() {
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
	eventMetadata, err := json.Marshal(map[string]any{
		"user_id":    input.UserID,
		"request_id": input.RequestID,
	})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化任务事件元数据失败", err)
	}
	if _, err := queries.CreateAgentEvent(ctx, db.CreateAgentEventParams{
		TenantID: input.TenantID,
		TaskID:   taskID,
		Type:     taskEventCreated,
		Input:    eventInput,
		Output:   json.RawMessage(`null`),
		Metadata: eventMetadata,
		TraceID:  &traceID,
	}); err != nil {
		return CreatedTask{}, mapPostgresWriteError(err)
	}

	outboxPayload, err := json.Marshal(map[string]any{
		"task_id":    taskID,
		"tenant_id":  input.TenantID,
		"user_id":    input.UserID,
		"trace_id":   traceID,
		"request_id": input.RequestID,
	})
	if err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeInternal, "序列化 outbox payload 失败", err)
	}
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

	if err := tx.Commit(ctx); err != nil {
		return CreatedTask{}, apperrors.Wrap(apperrors.CodeUnavailable, "提交任务创建事务失败", err)
	}

	return CreatedTask{
		TaskID:    task.TaskID,
		Status:    domaintask.StatusForAPI(string(task.Status)),
		CreatedAt: pgTime(task.CreatedAt),
	}, nil
}

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

	return taskDetailFromDB(task, currentStep), nil
}

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

	limit := int32(pageSize + 1)
	offset := int32((page - 1) * pageSize)
	canListTenant := authz.CanListTenantTasks(authn.User{TenantID: input.TenantID, UserID: input.UserID, Role: input.UserRole})

	var tasks []db.AgentTask
	if canListTenant {
		if status != "" {
			tasks, err = s.queries.ListAgentTasksByTenantAndStatus(ctx, db.ListAgentTasksByTenantAndStatusParams{
				TenantID:   input.TenantID,
				Status:     db.TaskStatus(status),
				OffsetRows: offset,
				LimitRows:  limit,
			})
		} else {
			tasks, err = s.queries.ListAgentTasksByTenant(ctx, db.ListAgentTasksByTenantParams{
				TenantID:   input.TenantID,
				OffsetRows: offset,
				LimitRows:  limit,
			})
		}
	} else {
		if status != "" {
			tasks, err = s.queries.ListAgentTasksByUserAndStatus(ctx, db.ListAgentTasksByUserAndStatusParams{
				TenantID:   input.TenantID,
				UserID:     input.UserID,
				Status:     db.TaskStatus(status),
				OffsetRows: offset,
				LimitRows:  limit,
			})
		} else {
			tasks, err = s.queries.ListAgentTasksByUser(ctx, db.ListAgentTasksByUserParams{
				TenantID:   input.TenantID,
				UserID:     input.UserID,
				OffsetRows: offset,
				LimitRows:  limit,
			})
		}
	}
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

func taskSummaryFromDB(task db.AgentTask) TaskSummary {
	return TaskSummary{
		TaskID:      task.TaskID,
		Goal:        task.Goal,
		Status:      domaintask.StatusForAPI(string(task.Status)),
		Budget:      task.Budget,
		BudgetUsage: task.BudgetUsage,
		TraceID:     task.TraceID,
		CreatedAt:   pgTime(task.CreatedAt),
		UpdatedAt:   pgTime(task.UpdatedAt),
	}
}

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
	if (page-1)*pageSize > 2147483647 {
		return 0, 0, apperrors.New(apperrors.CodeInvalidArg, "分页 offset 超出支持范围")
	}
	return page, pageSize, nil
}

func mapPostgresReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.New(apperrors.CodeNotFound, "任务不存在")
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库读取失败", err)
}

func mapPostgresWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return apperrors.New(apperrors.CodeForbidden, "用户不属于租户或租户不存在")
		case "23505":
			return apperrors.New(apperrors.CodeConflict, "任务资源已存在")
		}
	}
	return apperrors.Wrap(apperrors.CodeUnavailable, "数据库写入失败", err)
}

func pgTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}
