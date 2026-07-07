-- name: CreateArtifact :one
insert into artifacts (
    artifact_id,
    tenant_id,
    task_id,
    user_id,
    type,
    name,
    media_type,
    storage_backend,
    storage_key,
    content_json,
    size_bytes,
    checksum_sha256,
    metadata,
    status
) values (
    sqlc.arg(artifact_id),
    sqlc.arg(tenant_id),
    sqlc.arg(task_id),
    sqlc.arg(user_id),
    sqlc.arg(type),
    sqlc.arg(name),
    sqlc.narg(media_type),
    sqlc.arg(storage_backend),
    sqlc.narg(storage_key),
    sqlc.narg(content_json),
    sqlc.arg(size_bytes),
    sqlc.narg(checksum_sha256),
    sqlc.arg(metadata),
    sqlc.arg(status)
)
returning *;

-- name: GetArtifact :one
select *
from artifacts
where tenant_id = sqlc.arg(tenant_id)
  and artifact_id = sqlc.arg(artifact_id)
limit 1;

-- name: ListArtifactsByTask :many
select *
from artifacts
where tenant_id = sqlc.arg(tenant_id)
  and task_id = sqlc.arg(task_id)
  and status = 'AVAILABLE'
order by created_at desc, artifact_id desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: MarkArtifactDeleted :one
update artifacts
set status = 'DELETED',
    updated_at = now()
where tenant_id = sqlc.arg(tenant_id)
  and artifact_id = sqlc.arg(artifact_id)
returning *;
