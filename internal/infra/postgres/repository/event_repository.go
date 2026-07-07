package repository

import (
	"context"

	"agent-runtime/internal/infra/postgres/db"
)

type EventQueries interface {
	CreateAgentEvent(ctx context.Context, arg db.CreateAgentEventParams) (db.AgentEvent, error)
	GetAgentEvent(ctx context.Context, arg db.GetAgentEventParams) (db.AgentEvent, error)
	ListAgentEventsByTask(ctx context.Context, arg db.ListAgentEventsByTaskParams) ([]db.AgentEvent, error)
	ListAgentEventsByType(ctx context.Context, arg db.ListAgentEventsByTypeParams) ([]db.AgentEvent, error)
}

type EventRepository struct {
	queries EventQueries
}

func NewEventRepository(queries EventQueries) *EventRepository {
	return &EventRepository{queries: queries}
}

func (r *EventRepository) Append(ctx context.Context, params db.CreateAgentEventParams) (db.AgentEvent, error) {
	return r.queries.CreateAgentEvent(ctx, params)
}

func (r *EventRepository) Get(ctx context.Context, params db.GetAgentEventParams) (db.AgentEvent, error) {
	event, err := r.queries.GetAgentEvent(ctx, params)
	return event, mapNotFound(err)
}

func (r *EventRepository) ListByTask(ctx context.Context, params db.ListAgentEventsByTaskParams) ([]db.AgentEvent, error) {
	return r.queries.ListAgentEventsByTask(ctx, params)
}

func (r *EventRepository) ListByType(ctx context.Context, params db.ListAgentEventsByTypeParams) ([]db.AgentEvent, error) {
	return r.queries.ListAgentEventsByType(ctx, params)
}
