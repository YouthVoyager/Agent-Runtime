package repository

import (
	"context"

	"agent-runtime/internal/infra/postgres/db"
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

func NewToolCallRepository(queries ToolCallQueries) *ToolCallRepository {
	return &ToolCallRepository{queries: queries}
}

func (r *ToolCallRepository) Create(ctx context.Context, params db.CreateToolCallParams) (db.ToolCall, error) {
	return r.queries.CreateToolCall(ctx, params)
}

func (r *ToolCallRepository) Get(ctx context.Context, tenantID string, callID string) (db.ToolCall, error) {
	call, err := r.queries.GetToolCall(ctx, db.GetToolCallParams{
		TenantID: tenantID,
		CallID:   callID,
	})
	return call, mapNotFound(err)
}

func (r *ToolCallRepository) GetByIdempotencyKey(ctx context.Context, tenantID string, idempotencyKey string) (db.ToolCall, error) {
	call, err := r.queries.GetToolCallByIdempotencyKey(ctx, db.GetToolCallByIdempotencyKeyParams{
		TenantID:       tenantID,
		IdempotencyKey: idempotencyKey,
	})
	return call, mapNotFound(err)
}

func (r *ToolCallRepository) ListByTask(ctx context.Context, params db.ListToolCallsByTaskParams) ([]db.ToolCall, error) {
	return r.queries.ListToolCallsByTask(ctx, params)
}

func (r *ToolCallRepository) Start(ctx context.Context, params db.StartToolCallParams) (db.ToolCall, error) {
	call, err := r.queries.StartToolCall(ctx, params)
	return call, mapNotFound(err)
}

func (r *ToolCallRepository) Complete(ctx context.Context, params db.CompleteToolCallParams) (db.ToolCall, error) {
	call, err := r.queries.CompleteToolCall(ctx, params)
	return call, mapNotFound(err)
}

func (r *ToolCallRepository) Fail(ctx context.Context, params db.FailToolCallParams) (db.ToolCall, error) {
	call, err := r.queries.FailToolCall(ctx, params)
	return call, mapNotFound(err)
}

func (r *ToolCallRepository) UpdateApprovalStatus(ctx context.Context, params db.UpdateToolCallApprovalStatusParams) (db.ToolCall, error) {
	call, err := r.queries.UpdateToolCallApprovalStatus(ctx, params)
	return call, mapNotFound(err)
}
