package repository

import (
	"context"

	"stableagent/internal/infra/postgres/db"
)

type StateQueries interface {
	CreateAgentState(ctx context.Context, arg db.CreateAgentStateParams) (db.AgentState, error)
	GetAgentState(ctx context.Context, arg db.GetAgentStateParams) (db.AgentState, error)
	UpdateAgentStateOptimistic(ctx context.Context, arg db.UpdateAgentStateOptimisticParams) (db.AgentState, error)
}

type StateRepository struct {
	queries StateQueries
}

// NewStateRepository 创建 Agent 状态仓储。
func NewStateRepository(queries StateQueries) *StateRepository {
	return &StateRepository{queries: queries}
}

// Create 新建 Agent 状态记录。
func (r *StateRepository) Create(ctx context.Context, params db.CreateAgentStateParams) (db.AgentState, error) {
	return r.queries.CreateAgentState(ctx, params)
}

// Get 按租户和任务 ID 查询 Agent 状态，不存在时返回 ErrNotFound。
func (r *StateRepository) Get(ctx context.Context, tenantID string, taskID string) (db.AgentState, error) {
	state, err := r.queries.GetAgentState(ctx, db.GetAgentStateParams{
		TenantID: tenantID,
		TaskID:   taskID,
	})
	return state, mapNotFound(err)
}

// UpdateOptimistic 按版本号乐观更新 Agent 状态，版本不匹配时返回 ErrVersionConflict。
func (r *StateRepository) UpdateOptimistic(ctx context.Context, params db.UpdateAgentStateOptimisticParams) (db.AgentState, error) {
	state, err := r.queries.UpdateAgentStateOptimistic(ctx, params)
	return state, mapVersionConflict(err)
}
