-- name: CreateCheckpoint :one
insert into checkpoints (
    checkpoint_id,
    tenant_id,
    task_id,
    state_version,
    event_offset,
    state_snapshot,
    reason,
    trace_id
) values (
    sqlc.arg(checkpoint_id),
    sqlc.arg(tenant_id),
    sqlc.arg(task_id),
    sqlc.arg(state_version),
    sqlc.arg(event_offset),
    sqlc.arg(state_snapshot),
    sqlc.arg(reason),
    sqlc.narg(trace_id)
)
returning *;

-- name: GetCheckpoint :one
select *
from checkpoints
where tenant_id = sqlc.arg(tenant_id)
  and checkpoint_id = sqlc.arg(checkpoint_id)
limit 1;

-- name: GetLatestCheckpoint :one
select *
from checkpoints
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
order by created_at desc, checkpoint_id desc
limit 1;

-- name: ListCheckpointsByTask :many
select *
from checkpoints
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
order by created_at desc, checkpoint_id desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);
