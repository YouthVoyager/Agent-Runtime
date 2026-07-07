-- name: CreateToolCall :one
insert into tool_calls (
    call_id,
    tenant_id,
    task_id,
    user_id,
    tool_name,
    arguments,
    arguments_hash,
    idempotency_key,
    status,
    risk_level,
    approval_status
) values (
    sqlc.arg(call_id),
    sqlc.arg(tenant_id),
    sqlc.arg(task_id),
    sqlc.arg(user_id),
    sqlc.arg(tool_name),
    sqlc.arg(arguments),
    sqlc.arg(arguments_hash),
    sqlc.arg(idempotency_key),
    sqlc.arg(status),
    sqlc.arg(risk_level),
    sqlc.arg(approval_status)
)
returning *;

-- name: GetToolCall :one
select *
from tool_calls
where tenant_id = sqlc.arg(tenant_id)
  and call_id = sqlc.arg(call_id)
limit 1;

-- name: GetToolCallByIdempotencyKey :one
select *
from tool_calls
where tenant_id = sqlc.arg(tenant_id)
  and idempotency_key = sqlc.arg(idempotency_key)
limit 1;

-- name: ListToolCallsByTask :many
select *
from tool_calls
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
order by created_at asc, call_id asc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: StartToolCall :one
update tool_calls
set status = 'RUNNING',
    started_at = coalesce(started_at, now()),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and call_id = sqlc.arg(call_id)
returning *;

-- name: CompleteToolCall :one
update tool_calls
set status = 'SUCCEEDED',
    result = sqlc.narg(result),
    result_artifact_id = sqlc.narg(result_artifact_id),
    finished_at = now(),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and call_id = sqlc.arg(call_id)
returning *;

-- name: FailToolCall :one
update tool_calls
set status = 'FAILED',
    error_code = sqlc.arg(error_code),
    error_message = sqlc.arg(error_message),
    finished_at = now(),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and call_id = sqlc.arg(call_id)
returning *;

-- name: UpdateToolCallApprovalStatus :one
update tool_calls
set approval_status = sqlc.arg(approval_status),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and call_id = sqlc.arg(call_id)
returning *;
