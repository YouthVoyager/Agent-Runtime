package repository

import (
	"context"

	"stableagent/internal/infra/postgres/db"
)

type TaskQueries interface {
	CreateAgentTask(ctx context.Context, arg db.CreateAgentTaskParams) (db.AgentTask, error)
	GetAgentTask(ctx context.Context, arg db.GetAgentTaskParams) (db.AgentTask, error)
	ListAgentTasksByTenant(ctx context.Context, arg db.ListAgentTasksByTenantParams) ([]db.AgentTask, error)
	ListAgentTasksByUserAndStatus(ctx context.Context, arg db.ListAgentTasksByUserAndStatusParams) ([]db.AgentTask, error)
	UpdateAgentTaskStatus(ctx context.Context, arg db.UpdateAgentTaskStatusParams) (db.AgentTask, error)
	UpdateAgentTaskWorkflow(ctx context.Context, arg db.UpdateAgentTaskWorkflowParams) (db.AgentTask, error)
	UpdateAgentTaskBudgetUsage(ctx context.Context, arg db.UpdateAgentTaskBudgetUsageParams) (db.AgentTask, error)
	MarkAgentTaskFailed(ctx context.Context, arg db.MarkAgentTaskFailedParams) (db.AgentTask, error)
}

type TaskRepository struct {
	queries TaskQueries
}

// NewTaskRepository 创建任务仓储。
func NewTaskRepository(queries TaskQueries) *TaskRepository {
	return &TaskRepository{queries: queries}
}

// Create 新建任务记录。
func (r *TaskRepository) Create(ctx context.Context, params db.CreateAgentTaskParams) (db.AgentTask, error) {
	return r.queries.CreateAgentTask(ctx, params)
}

// Get 按租户和任务 ID 查询任务，不存在时返回 ErrNotFound。
func (r *TaskRepository) Get(ctx context.Context, tenantID string, taskID string) (db.AgentTask, error) {
	task, err := r.queries.GetAgentTask(ctx, db.GetAgentTaskParams{
		TenantID: tenantID,
		TaskID:   taskID,
	})
	return task, mapNotFound(err)
}

// ListByTenant 查询租户内任务列表。
func (r *TaskRepository) ListByTenant(ctx context.Context, params db.ListAgentTasksByTenantParams) ([]db.AgentTask, error) {
	return r.queries.ListAgentTasksByTenant(ctx, params)
}

// ListByUserAndStatus 查询指定用户在指定状态下的任务列表。
func (r *TaskRepository) ListByUserAndStatus(ctx context.Context, params db.ListAgentTasksByUserAndStatusParams) ([]db.AgentTask, error) {
	return r.queries.ListAgentTasksByUserAndStatus(ctx, params)
}

// UpdateStatus 更新任务状态，不存在时返回 ErrNotFound。
func (r *TaskRepository) UpdateStatus(ctx context.Context, params db.UpdateAgentTaskStatusParams) (db.AgentTask, error) {
	task, err := r.queries.UpdateAgentTaskStatus(ctx, params)
	return task, mapNotFound(err)
}

// UpdateWorkflow 回填任务关联的 workflow ID，不存在时返回 ErrNotFound。
func (r *TaskRepository) UpdateWorkflow(ctx context.Context, params db.UpdateAgentTaskWorkflowParams) (db.AgentTask, error) {
	task, err := r.queries.UpdateAgentTaskWorkflow(ctx, params)
	return task, mapNotFound(err)
}

// UpdateBudgetUsage 更新任务预算使用量，不存在时返回 ErrNotFound。
func (r *TaskRepository) UpdateBudgetUsage(ctx context.Context, params db.UpdateAgentTaskBudgetUsageParams) (db.AgentTask, error) {
	task, err := r.queries.UpdateAgentTaskBudgetUsage(ctx, params)
	return task, mapNotFound(err)
}

// MarkFailed 标记任务失败并保存最后错误信息。
func (r *TaskRepository) MarkFailed(ctx context.Context, params db.MarkAgentTaskFailedParams) (db.AgentTask, error) {
	task, err := r.queries.MarkAgentTaskFailed(ctx, params)
	return task, mapNotFound(err)
}
