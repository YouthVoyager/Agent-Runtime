-- name: CreateApproval :one
insert into approvals (
    approval_id,
    tenant_id,
    task_id,
    call_id,
    approver_id,
    status,
    risk_level,
    approval_reason,
    comment,
    expires_at
) values (
    sqlc.arg(approval_id),
    sqlc.arg(tenant_id),
    sqlc.arg(task_id),
    sqlc.arg(call_id),
    sqlc.narg(approver_id),
    sqlc.arg(status),
    sqlc.arg(risk_level),
    sqlc.narg(approval_reason),
    sqlc.narg(comment),
    sqlc.narg(expires_at)
)
returning *;

-- name: GetApprovalByCall :one
select *
from approvals
where tenant_id = sqlc.arg(tenant_id)
  and call_id = sqlc.arg(call_id)
limit 1;

-- name: DecideApproval :one
update approvals
set status = sqlc.arg(status),
    approver_id = sqlc.arg(approver_id),
    comment = sqlc.narg(comment),
    decided_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and approval_id = sqlc.arg(approval_id)
returning *;
