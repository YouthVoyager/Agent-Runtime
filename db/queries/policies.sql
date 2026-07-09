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
-- worker 侧执行期策略校验只使用 role = '' 的租户默认策略,按角色维度的精细策略由管理台 API 层处理。
select *
from tenant_tool_policies
where tenant_id = sqlc.arg(tenant_id)
  and tool_name = sqlc.arg(tool_name)
  and role = ''
limit 1;

-- name: ListTenantToolPolicies :many
select *
from tenant_tool_policies
where tenant_id = sqlc.arg(tenant_id)
order by tool_name asc;

-- name: CreateToolPolicy :one
insert into tenant_tool_policies (
    policy_id,
    tenant_id,
    tool_name,
    role,
    effect,
    max_risk_level,
    require_approval,
    max_calls_per_day,
    argument_rules,
    timeout_seconds,
    config
) values (
    sqlc.arg(policy_id),
    sqlc.arg(tenant_id),
    sqlc.arg(tool_name),
    sqlc.arg(role),
    sqlc.arg(effect),
    sqlc.arg(max_risk_level),
    sqlc.arg(require_approval),
    sqlc.narg(max_calls_per_day),
    sqlc.arg(argument_rules),
    sqlc.narg(timeout_seconds),
    '{}'::jsonb
)
on conflict (tenant_id, tool_name, role) do update
set effect = excluded.effect,
    max_risk_level = excluded.max_risk_level,
    require_approval = excluded.require_approval,
    max_calls_per_day = excluded.max_calls_per_day,
    argument_rules = excluded.argument_rules,
    timeout_seconds = excluded.timeout_seconds,
    updated_at = now()
returning *;

-- name: GetToolPolicyByID :one
select *
from tenant_tool_policies
where tenant_id = sqlc.arg(tenant_id)
  and policy_id = sqlc.arg(policy_id)
limit 1;

-- name: ListToolPoliciesFiltered :many
select *
from tenant_tool_policies
where tenant_id = sqlc.arg(tenant_id)
  and (sqlc.arg(tool_name)::text = '' or tool_name = sqlc.arg(tool_name))
  and (sqlc.arg(role)::text = '' or role = sqlc.arg(role))
order by tool_name asc, role asc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: ListToolPoliciesForToolRole :many
-- 策略模拟器按 tenant + tool 查询候选策略,再由应用层按角色精确匹配优先。
select *
from tenant_tool_policies
where tenant_id = sqlc.arg(tenant_id)
  and tool_name = sqlc.arg(tool_name)
  and (role = sqlc.arg(role) or role = '')
order by role desc
limit 5;

-- name: UpdateToolPolicyByID :one
update tenant_tool_policies
set effect = sqlc.arg(effect),
    max_risk_level = sqlc.arg(max_risk_level),
    require_approval = sqlc.arg(require_approval),
    max_calls_per_day = sqlc.narg(max_calls_per_day),
    argument_rules = sqlc.arg(argument_rules),
    timeout_seconds = sqlc.narg(timeout_seconds),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and policy_id = sqlc.arg(policy_id)
returning *;

-- name: DisableToolPolicyByID :one
update tenant_tool_policies
set effect = 'DENY',
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and policy_id = sqlc.arg(policy_id)
returning *;
