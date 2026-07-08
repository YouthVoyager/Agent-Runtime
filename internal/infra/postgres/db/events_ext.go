package db

import "context"

const listAgentEventsByTaskAndType = `-- name: ListAgentEventsByTaskAndType :many
select event_id, task_id, tenant_id, type, input, output, metadata, trace_id, span_id, created_at
from agent_events
where tenant_id = $1
  and task_id = $2
  and type = $3
  and event_id > $4
order by event_id asc
limit $5
`

type ListAgentEventsByTaskAndTypeParams struct {
	TenantID     string `db:"tenant_id" json:"tenant_id"`
	TaskID       string `db:"task_id" json:"task_id"`
	Type         string `db:"type" json:"type"`
	AfterEventID int64  `db:"after_event_id" json:"after_event_id"`
	LimitRows    int32  `db:"limit_rows" json:"limit_rows"`
}

// ListAgentEventsByTaskAndType 按任务、事件类型和 event_id 游标顺序读取 timeline。
func (q *Queries) ListAgentEventsByTaskAndType(ctx context.Context, arg ListAgentEventsByTaskAndTypeParams) ([]AgentEvent, error) {
	rows, err := q.db.Query(ctx, listAgentEventsByTaskAndType,
		arg.TenantID,
		arg.TaskID,
		arg.Type,
		arg.AfterEventID,
		arg.LimitRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []AgentEvent{}
	for rows.Next() {
		var event AgentEvent
		if err := rows.Scan(
			&event.EventID,
			&event.TaskID,
			&event.TenantID,
			&event.Type,
			&event.Input,
			&event.Output,
			&event.Metadata,
			&event.TraceID,
			&event.SpanID,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
