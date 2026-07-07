-- name: CreateAgentState :one
insert into agent_states (
    tenant_id,
    task_id,
    current_plan,
    current_step,
    memory_summary,
    constraints,
    artifacts,
    loop_fingerprints
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(task_id),
    sqlc.arg(current_plan),
    sqlc.arg(current_step),
    sqlc.narg(memory_summary),
    sqlc.arg(constraints),
    sqlc.arg(artifacts),
    sqlc.arg(loop_fingerprints)
)
returning *;

-- name: GetAgentState :one
select *
from agent_states
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
limit 1;

-- name: UpdateAgentStateOptimistic :one
update agent_states
set current_plan = sqlc.arg(current_plan),
    current_step = sqlc.arg(current_step),
    memory_summary = sqlc.narg(memory_summary),
    constraints = sqlc.arg(constraints),
    artifacts = sqlc.arg(artifacts),
    loop_fingerprints = sqlc.arg(loop_fingerprints),
    version = version + 1,
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
  and version = sqlc.arg(version)
returning *;
