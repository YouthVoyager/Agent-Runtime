package repository

import (
	"context"

	"stableagent/internal/infra/postgres/db"
)

type EventQueries interface {
	CreateAgentEvent(ctx context.Context, arg db.CreateAgentEventParams) (db.AgentEvent, error)
	GetAgentEvent(ctx context.Context, arg db.GetAgentEventParams) (db.AgentEvent, error)
	ListAgentEventsByTask(ctx context.Context, arg db.ListAgentEventsByTaskParams) ([]db.AgentEvent, error)
	ListAgentEventsByTaskAndType(ctx context.Context, arg db.ListAgentEventsByTaskAndTypeParams) ([]db.AgentEvent, error)
	ListAgentEventsByType(ctx context.Context, arg db.ListAgentEventsByTypeParams) ([]db.AgentEvent, error)
}

type EventRepository struct {
	queries EventQueries
}

// NewEventRepository 创建事件仓储。
func NewEventRepository(queries EventQueries) *EventRepository {
	return &EventRepository{queries: queries}
}

// Append 追加 Agent 执行事件。
func (r *EventRepository) Append(ctx context.Context, params db.CreateAgentEventParams) (db.AgentEvent, error) {
	return r.queries.CreateAgentEvent(ctx, params)
}

// Get 查询单条事件，不存在时返回 ErrNotFound。
func (r *EventRepository) Get(ctx context.Context, params db.GetAgentEventParams) (db.AgentEvent, error) {
	event, err := r.queries.GetAgentEvent(ctx, params)
	return event, mapNotFound(err)
}

// ListByTask 查询任务维度的事件时间线。
func (r *EventRepository) ListByTask(ctx context.Context, params db.ListAgentEventsByTaskParams) ([]db.AgentEvent, error) {
	return r.queries.ListAgentEventsByTask(ctx, params)
}

// ListByTaskAndType 查询任务维度下指定事件类型的时间线。
func (r *EventRepository) ListByTaskAndType(ctx context.Context, params db.ListAgentEventsByTaskAndTypeParams) ([]db.AgentEvent, error) {
	return r.queries.ListAgentEventsByTaskAndType(ctx, params)
}

// ListByType 查询指定类型的事件列表。
func (r *EventRepository) ListByType(ctx context.Context, params db.ListAgentEventsByTypeParams) ([]db.AgentEvent, error) {
	return r.queries.ListAgentEventsByType(ctx, params)
}
