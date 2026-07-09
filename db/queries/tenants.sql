-- name: CreateTenant :one
insert into tenants (
    tenant_id,
    name,
    status,
    metadata
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(name),
    sqlc.arg(status),
    sqlc.arg(metadata)
)
returning *;

-- name: GetTenant :one
select *
from tenants
where tenant_id = sqlc.arg(tenant_id)
limit 1;

-- name: CreateUser :one
insert into users (
    tenant_id,
    user_id,
    email,
    display_name,
    role,
    status,
    metadata
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(user_id),
    sqlc.arg(email),
    sqlc.narg(display_name),
    sqlc.arg(role),
    sqlc.arg(status),
    sqlc.arg(metadata)
)
returning *;

-- name: GetUser :one
select *
from users
where tenant_id = sqlc.arg(tenant_id)
  and user_id = sqlc.arg(user_id)
limit 1;

-- name: CreateTenantFull :one
insert into tenants (
    tenant_id,
    name,
    status,
    metadata,
    config
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(name),
    sqlc.arg(status),
    sqlc.arg(metadata),
    sqlc.arg(config)
)
returning *;

-- name: ListTenants :many
select *
from tenants
order by created_at desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: UpdateTenant :one
update tenants
set name = sqlc.arg(name),
    status = sqlc.arg(status),
    config = sqlc.arg(config),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
returning *;

-- name: TenantUsageToday :one
select
    count(distinct agent_tasks.task_id) filter (where agent_tasks.created_at >= sqlc.arg(since))::bigint as tasks_today,
    coalesce(sum((agent_tasks.budget_usage ->> 'used_tokens')::bigint) filter (where agent_tasks.created_at >= sqlc.arg(since)), 0)::bigint as tokens_today,
    coalesce(sum((agent_tasks.budget_usage ->> 'used_cost_usd')::float8) filter (where agent_tasks.created_at >= sqlc.arg(since)), 0)::float8 as cost_today_usd
from agent_tasks
where agent_tasks.tenant_id = sqlc.arg(tenant_id);

-- name: ListUsersByTenant :many
select *
from users
where tenant_id = sqlc.arg(tenant_id)
order by created_at desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: ListAllUsers :many
select *
from users
order by created_at desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: CreateInvitedUser :one
insert into users (
    tenant_id,
    user_id,
    email,
    display_name,
    role,
    status,
    metadata
) values (
    sqlc.arg(tenant_id),
    sqlc.arg(user_id),
    sqlc.arg(email),
    sqlc.narg(display_name),
    sqlc.arg(role),
    'ACTIVE',
    '{}'::jsonb
)
returning *;

-- name: UpdateUser :one
update users
set role = sqlc.arg(role),
    status = sqlc.arg(status),
    display_name = sqlc.narg(display_name),
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and user_id = sqlc.arg(user_id)
returning *;
