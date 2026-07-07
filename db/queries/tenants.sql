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
