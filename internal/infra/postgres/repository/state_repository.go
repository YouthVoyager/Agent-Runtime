package repository

import (
	"context"

	"agent-runtime/internal/infra/postgres/db"
)

type StateQueries interface {
	CreateAgentState(ctx context.Context, arg db.CreateAgentStateParams) (db.AgentState, error)
	GetAgentState(ctx context.Context, arg db.GetAgentStateParams) (db.AgentState, error)
	UpdateAgentStateOptimistic(ctx context.Context, arg db.UpdateAgentStateOptimisticParams) (db.AgentState, error)
}

type StateRepository struct {
	queries StateQueries
}

func NewStateRepository(queries StateQueries) *StateRepository {
	return &StateRepository{queries: queries}
}

func (r *StateRepository) Create(ctx context.Context, params db.CreateAgentStateParams) (db.AgentState, error) {
	return r.queries.CreateAgentState(ctx, params)
}

func (r *StateRepository) Get(ctx context.Context, tenantID string, taskID string) (db.AgentState, error) {
	state, err := r.queries.GetAgentState(ctx, db.GetAgentStateParams{
		TenantID: tenantID,
		TaskID:   taskID,
	})
	return state, mapNotFound(err)
}

func (r *StateRepository) UpdateOptimistic(ctx context.Context, params db.UpdateAgentStateOptimisticParams) (db.AgentState, error) {
	state, err := r.queries.UpdateAgentStateOptimistic(ctx, params)
	return state, mapVersionConflict(err)
}
