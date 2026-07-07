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

func NewCheckpointRepository(queries CheckpointQueries) *CheckpointRepository {
	return &CheckpointRepository{queries: queries}
}

func (r *CheckpointRepository) Create(ctx context.Context, params db.CreateCheckpointParams) (db.Checkpoint, error) {
	return r.queries.CreateCheckpoint(ctx, params)
}

func (r *CheckpointRepository) Get(ctx context.Context, tenantID string, checkpointID string) (db.Checkpoint, error) {
	checkpoint, err := r.queries.GetCheckpoint(ctx, db.GetCheckpointParams{
		TenantID:     tenantID,
		CheckpointID: checkpointID,
	})
	return checkpoint, mapNotFound(err)
}

func (r *CheckpointRepository) Latest(ctx context.Context, tenantID string, taskID string) (db.Checkpoint, error) {
	checkpoint, err := r.queries.GetLatestCheckpoint(ctx, db.GetLatestCheckpointParams{
		TenantID: tenantID,
		TaskID:   taskID,
	})
	return checkpoint, mapNotFound(err)
}

func (r *CheckpointRepository) ListByTask(ctx context.Context, params db.ListCheckpointsByTaskParams) ([]db.Checkpoint, error) {
	return r.queries.ListCheckpointsByTask(ctx, params)
}
