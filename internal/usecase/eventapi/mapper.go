package eventapi

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domainevent "agent-runtime/internal/domain/event"
	"agent-runtime/internal/infra/postgres/db"
)

// eventFromDB 将数据库事件记录转换为 API 可返回的领域事件。
func eventFromDB(row db.AgentEvent) domainevent.AgentEvent {
	return domainevent.AgentEvent{
		EventID:   row.EventID,
		TaskID:    row.TaskID,
		TenantID:  row.TenantID,
		Type:      row.Type,
		Input:     domainevent.NormalizeJSONValue(row.Input),
		Output:    domainevent.NormalizeJSONValue(row.Output),
		Metadata:  domainevent.NormalizeMetadata(row.Metadata),
		TraceID:   row.TraceID,
		SpanID:    row.SpanID,
		CreatedAt: pgTime(row.CreatedAt),
	}
}

// pgTime 将 pgx 时间类型转换为 UTC time.Time，空值返回零时间。
func pgTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}
