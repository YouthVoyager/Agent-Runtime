-- name: CreateTaskTemplate :one
insert into task_templates (
    template_id,
    tenant_id,
    name,
    payload,
    shared,
    created_by
) values (
    sqlc.arg(template_id),
    sqlc.arg(tenant_id),
    sqlc.arg(name),
    sqlc.arg(payload),
    sqlc.arg(shared),
    sqlc.arg(created_by)
)
returning *;

-- name: GetTaskTemplate :one
select *
from task_templates
where tenant_id = sqlc.arg(tenant_id)
  and template_id = sqlc.arg(template_id)
limit 1;

-- name: ListTaskTemplatesForUser :many
select *
from task_templates
where tenant_id = sqlc.arg(tenant_id)
  and (created_by = sqlc.arg(created_by) or shared = true)
order by created_at desc
limit sqlc.arg(limit_rows)
offset sqlc.arg(offset_rows);

-- name: DeleteTaskTemplate :execrows
delete from task_templates
where tenant_id = sqlc.arg(tenant_id)
  and template_id = sqlc.arg(template_id);
