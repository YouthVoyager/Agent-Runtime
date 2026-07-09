-- name: UpsertSystemSetting :one
insert into system_settings (section, value, updated_by)
values (sqlc.arg(section), sqlc.arg(value), sqlc.narg(updated_by))
on conflict (section) do update
set value = excluded.value,
    updated_by = excluded.updated_by,
    updated_at = now()
returning *;

-- name: GetSystemSetting :one
select *
from system_settings
where section = sqlc.arg(section)
limit 1;

-- name: ListSystemSettings :many
select *
from system_settings
order by section asc;
