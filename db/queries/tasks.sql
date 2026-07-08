-- name: CreateAgentTask :one
insert into agent_tasks (
    task_id,
    tenant_id,
    user_id,
    goal,
    status,
    budget,
    budget_usage,
    trace_id
) values (
    sqlc.arg(task_id),
    sqlc.arg(tenant_id),
    sqlc.arg(user_id),
    sqlc.arg(goal),
    sqlc.arg(status),
    sqlc.arg(budget),
    sqlc.arg(budget_usage),
    sqlc.narg(trace_id)
)
returning *;

-- name: GetAgentTask :one
select *
from agent_tasks
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
limit 1;

-- name: ListAgentTasksByTenant :many
select *
from agent_tasks
where tenant_id = sqlc.arg(tenant_id)
order by created_at desc, task_id desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: ListAgentTasksByTenantAndStatus :many
select *
from agent_tasks
where tenant_id = sqlc.arg(tenant_id)
  and status = sqlc.arg(status)
order by created_at desc, task_id desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: ListAgentTasksByUser :many
select *
from agent_tasks
where tenant_id = sqlc.arg(tenant_id)
  and user_id = sqlc.arg(user_id)
order by created_at desc, task_id desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: ListAgentTasksByUserAndStatus :many
select *
from agent_tasks
where tenant_id = sqlc.arg(tenant_id)
  and user_id = sqlc.arg(user_id)
  and status = sqlc.arg(status)
order by created_at desc, task_id desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: UpdateAgentTaskStatus :one
update agent_tasks
set status = sqlc.arg(status),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
returning *;

-- name: UpdateAgentTaskWorkflow :one
update agent_tasks
set workflow_id = sqlc.narg(workflow_id),
    trace_id = sqlc.narg(trace_id),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
returning *;

-- name: UpdateAgentTaskBudgetUsage :one
update agent_tasks
set budget_usage = sqlc.arg(budget_usage),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
returning *;

-- name: MarkAgentTaskFailed :one
update agent_tasks
set status = 'FAILED',
    last_error_code = sqlc.arg(last_error_code),
    last_error_message = sqlc.arg(last_error_message),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
returning *;

-- name: CreateTaskIdempotencyKey :exec
insert into task_idempotency_keys (
    tenant_id,
    user_id,
    client_request_id,
    task_id
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(user_id),
    sqlc.arg(client_request_id),
    sqlc.arg(task_id)
);

-- name: GetTaskByClientRequestID :one
select agent_tasks.*
from task_idempotency_keys
join agent_tasks
  on agent_tasks.tenant_id = task_idempotency_keys.tenant_id
 and agent_tasks.task_id = task_idempotency_keys.task_id
where task_idempotency_keys.tenant_id = sqlc.arg(tenant_id)
  and task_idempotency_keys.user_id = sqlc.arg(user_id)
  and task_idempotency_keys.client_request_id = sqlc.arg(client_request_id)
limit 1;

-- name: UpdateAgentTaskStatusIfCurrent :one
update agent_tasks
set status = sqlc.arg(next_status),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
  and status = sqlc.arg(current_status)
returning *;

-- name: ListRunnableAgentTasks :many
select *
from agent_tasks
where status in ('QUEUED', 'CANCELING')
order by updated_at asc, task_id asc
limit sqlc.arg(limit_rows);
