package repository

import (
	"context"

	"agent-runtime/internal/infra/postgres/db"
)

type CheckpointQueries interface {
	CreateCheckpoint(ctx context.Context, arg db.CreateCheckpointParams) (db.Checkpoint, error)
	GetCheckpoint(ctx context.Context, arg db.GetCheckpointParams) (db.Checkpoint, error)
	GetLatestCheckpoint(ctx context.Context, arg db.GetLatestCheckpointParams) (db.Checkpoint, error)
	ListCheckpointsByTask(ctx context.Context, arg db.ListCheckpointsByTaskParams) ([]db.Checkpoint, error)
}

type CheckpointRepository struct {
	queries CheckpointQueries
}

// NewCheckpointRepository 创建 checkpoint 仓储。
func NewCheckpointRepository(queries CheckpointQueries) *CheckpointRepository {
	return &CheckpointRepository{queries: queries}
}

// Create 新建 checkpoint 记录。
func (r *CheckpointRepository) Create(ctx context.Context, params db.CreateCheckpointParams) (db.Checkpoint, error) {
	return r.queries.CreateCheckpoint(ctx, params)
}

// Get 按租户和 checkpoint ID 查询 checkpoint，不存在时返回 ErrNotFound。
func (r *CheckpointRepository) Get(ctx context.Context, tenantID string, checkpointID string) (db.Checkpoint, error) {
	checkpoint, err := r.queries.GetCheckpoint(ctx, db.GetCheckpointParams{
		TenantID:     tenantID,
		CheckpointID: checkpointID,
	})
	return checkpoint, mapNotFound(err)
}

// Latest 查询任务最新 checkpoint，不存在时返回 ErrNotFound。
func (r *CheckpointRepository) Latest(ctx context.Context, tenantID string, taskID string) (db.Checkpoint, error) {
	checkpoint, err := r.queries.GetLatestCheckpoint(ctx, db.GetLatestCheckpointParams{
		TenantID: tenantID,
		TaskID:   taskID,
	})
	return checkpoint, mapNotFound(err)
}

// ListByTask 查询任务下的 checkpoint 列表。
func (r *CheckpointRepository) ListByTask(ctx context.Context, params db.ListCheckpointsByTaskParams) ([]db.Checkpoint, error) {
	return r.queries.ListCheckpointsByTask(ctx, params)
}
