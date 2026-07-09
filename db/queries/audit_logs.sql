-- name: CreateAuditLog :one
insert into audit_logs (
    audit_id,
    actor_id,
    tenant_id,
    action,
    resource_type,
    resource_id,
    before,
    after,
    ip,
    user_agent
) values (
    sqlc.arg(audit_id),
    sqlc.arg(actor_id),
    sqlc.narg(tenant_id),
    sqlc.arg(action),
    sqlc.arg(resource_type),
    sqlc.narg(resource_id),
    sqlc.narg(before),
    sqlc.narg(after),
    sqlc.narg(ip),
    sqlc.narg(user_agent)
)
returning *;

-- name: GetAuditLog :one
select *
from audit_logs
where audit_id = sqlc.arg(audit_id)
limit 1;
