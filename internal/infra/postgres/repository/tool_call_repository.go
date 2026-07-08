package repository

import (
	"context"

	"stableagent/internal/infra/postgres/db"
)

type ToolCallQueries interface {
	CreateToolCall(ctx context.Context, arg db.CreateToolCallParams) (db.ToolCall, error)
	GetToolCall(ctx context.Context, arg db.GetToolCallParams) (db.ToolCall, error)
	GetToolCallByIdempotencyKey(ctx context.Context, arg db.GetToolCallByIdempotencyKeyParams) (db.ToolCall, error)
	ListToolCallsByTask(ctx context.Context, arg db.ListToolCallsByTaskParams) ([]db.ToolCall, error)
	StartToolCall(ctx context.Context, arg db.StartToolCallParams) (db.ToolCall, error)
	CompleteToolCall(ctx context.Context, arg db.CompleteToolCallParams) (db.ToolCall, error)
	FailToolCall(ctx context.Context, arg db.FailToolCallParams) (db.ToolCall, error)
	UpdateToolCallApprovalStatus(ctx context.Context, arg db.UpdateToolCallApprovalStatusParams) (db.ToolCall, error)
}

type ToolCallRepository struct {
	queries ToolCallQueries
}

// NewToolCallRepository 创建工具调用仓储。
func NewToolCallRepository(queries ToolCallQueries) *ToolCallRepository {
	return &ToolCallRepository{queries: queries}
}

// Create 新建工具调用记录。
func (r *ToolCallRepository) Create(ctx context.Context, params db.CreateToolCallParams) (db.ToolCall, error) {
	return r.queries.CreateToolCall(ctx, params)
}

// Get 按租户和调用 ID 查询工具调用，不存在时返回 ErrNotFound。
func (r *ToolCallRepository) Get(ctx context.Context, tenantID string, callID string) (db.ToolCall, error) {
	call, err := r.queries.GetToolCall(ctx, db.GetToolCallParams{
		TenantID: tenantID,
		CallID:   callID,
	})
	return call, mapNotFound(err)
}

// GetByIdempotencyKey 按幂等键查询工具调用，不存在时返回 ErrNotFound。
func (r *ToolCallRepository) GetByIdempotencyKey(ctx context.Context, tenantID string, idempotencyKey string) (db.ToolCall, error) {
	call, err := r.queries.GetToolCallByIdempotencyKey(ctx, db.GetToolCallByIdempotencyKeyParams{
		TenantID:       tenantID,
		IdempotencyKey: idempotencyKey,
	})
	return call, mapNotFound(err)
}

// ListByTask 查询任务下的工具调用列表。
func (r *ToolCallRepository) ListByTask(ctx context.Context, params db.ListToolCallsByTaskParams) ([]db.ToolCall, error) {
	return r.queries.ListToolCallsByTask(ctx, params)
}

// Start 将工具调用标记为执行中。
func (r *ToolCallRepository) Start(ctx context.Context, params db.StartToolCallParams) (db.ToolCall, error) {
	call, err := r.queries.StartToolCall(ctx, params)
	return call, mapNotFound(err)
}

// Complete 将工具调用标记为成功并保存输出。
func (r *ToolCallRepository) Complete(ctx context.Context, params db.CompleteToolCallParams) (db.ToolCall, error) {
	call, err := r.queries.CompleteToolCall(ctx, params)
	return call, mapNotFound(err)
}

// Fail 将工具调用标记为失败并保存错误信息。
func (r *ToolCallRepository) Fail(ctx context.Context, params db.FailToolCallParams) (db.ToolCall, error) {
	call, err := r.queries.FailToolCall(ctx, params)
	return call, mapNotFound(err)
}

// UpdateApprovalStatus 更新工具调用关联的审批状态。
func (r *ToolCallRepository) UpdateApprovalStatus(ctx context.Context, params db.UpdateToolCallApprovalStatusParams) (db.ToolCall, error) {
	call, err := r.queries.UpdateToolCallApprovalStatus(ctx, params)
	return call, mapNotFound(err)
}
