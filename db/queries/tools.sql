-- name: ListTools :many
select *
from tools
order by tool_name asc;

-- name: ListEnabledTools :many
select *
from tools
where enabled = true
order by tool_name asc;

-- name: GetTool :one
select *
from tools
where tool_name = sqlc.arg(tool_name)
limit 1;

-- name: UpdateTool :one
update tools
set enabled = sqlc.arg(enabled),
    default_risk_level = sqlc.arg(default_risk_level),
    timeout_seconds = sqlc.arg(timeout_seconds),
    description = sqlc.arg(description),
    category = sqlc.arg(category),
    params_schema = sqlc.arg(params_schema),
    retry_policy = sqlc.arg(retry_policy),
    updated_at = now()
where tool_name = sqlc.arg(tool_name)
returning *;

-- name: ToolCallStatsByTool :one
select
    count(*)::bigint as calls_total,
    coalesce(sum(case when status = 'FAILED' then 1 else 0 end), 0)::bigint as error_total,
    coalesce(avg(extract(epoch from (finished_at - started_at)) * 1000) filter (where finished_at is not null and started_at is not null), 0)::float8 as avg_duration_ms
from tool_calls
where tool_name = sqlc.arg(tool_name)
  and created_at >= sqlc.arg(since);
