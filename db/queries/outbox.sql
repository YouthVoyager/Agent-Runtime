-- name: CreateTaskOutboxMessage :one
insert into task_outbox (
    tenant_id,
    aggregate_type,
    aggregate_id,
    event_type,
    payload,
    status,
    next_retry_at
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(aggregate_type),
    sqlc.arg(aggregate_id),
    sqlc.arg(event_type),
    sqlc.arg(payload),
    sqlc.arg(status),
    sqlc.narg(next_retry_at)
)
returning *;

-- name: ListPendingTaskOutboxMessages :many
select *
from task_outbox
where status in ('PENDING', 'FAILED')
  and (next_retry_at is null or next_retry_at <= now())
order by created_at asc, outbox_id asc
limit sqlc.arg(limit_rows);

-- name: MarkTaskOutboxProcessing :one
update task_outbox
set status = 'PROCESSING',
    updated_at = now()
where outbox_id = sqlc.arg(outbox_id)
  and status in ('PENDING', 'FAILED')
returning *;

-- name: ResetStaleTaskOutboxProcessing :exec
update task_outbox
set status = 'FAILED',
    next_retry_at = now(),
    updated_at = now()
where status = 'PROCESSING'
  and updated_at < now() - sqlc.arg(stale_after)::interval;

-- name: MarkTaskOutboxSent :one
update task_outbox
set status = 'SENT',
    updated_at = now()
where outbox_id = sqlc.arg(outbox_id)
returning *;

-- name: MarkTaskOutboxFailed :one
update task_outbox
set status = sqlc.arg(status),
    retry_count = retry_count + 1,
    next_retry_at = sqlc.narg(next_retry_at),
    updated_at = now()
where outbox_id = sqlc.arg(outbox_id)
returning *;
