-- name: CreateAgentEvent :one
insert into agent_events (
    tenant_id,
    task_id,
    type,
    input,
    output,
    metadata,
    trace_id,
    span_id
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(task_id),
    sqlc.arg(type),
    sqlc.narg(input),
    sqlc.narg(output),
    sqlc.narg(metadata),
    sqlc.narg(trace_id),
    sqlc.narg(span_id)
)
returning *;

-- name: GetAgentEvent :one
select *
from agent_events
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
  and event_id = sqlc.arg(event_id)
limit 1;

-- name: ListAgentEventsByTask :many
select *
from agent_events
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
  and event_id > sqlc.arg(after_event_id)
order by event_id asc
limit sqlc.arg(limit_rows);

-- name: ListAgentEventsByTaskAndType :many
select *
from agent_events
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
  and type = sqlc.arg(type)
  and event_id > sqlc.arg(after_event_id)
order by event_id asc
limit sqlc.arg(limit_rows);

-- name: ListAgentEventsByType :many
select *
from agent_events
where tenant_id = sqlc.arg(tenant_id)
  and type = sqlc.arg(type)
order by created_at desc, event_id desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);
