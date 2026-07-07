-- name: UpsertTenantToolPolicy :one
insert into tenant_tool_policies (
    policy_id,
    tenant_id,
    tool_name,
    effect,
    max_risk_level,
    require_approval,
    config
) values (
    sqlc.arg(policy_id),
    sqlc.arg(tenant_id),
    sqlc.arg(tool_name),
    sqlc.arg(effect),
    sqlc.arg(max_risk_level),
    sqlc.arg(require_approval),
    sqlc.arg(config)
)
on conflict (tenant_id, tool_name) do update
set effect = excluded.effect,
    max_risk_level = excluded.max_risk_level,
    require_approval = excluded.require_approval,
    config = excluded.config,
    updated_at = now()
returning *;

-- name: GetTenantToolPolicy :one
select *
from tenant_tool_policies
where tenant_id = sqlc.arg(tenant_id)
  and tool_name = sqlc.arg(tool_name)
limit 1;

-- name: ListTenantToolPolicies :many
select *
from tenant_tool_policies
where tenant_id = sqlc.arg(tenant_id)
order by tool_name asc;
