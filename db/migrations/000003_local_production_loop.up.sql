-- 本地生产闭环补充：任务创建幂等键、默认本地租户和用户。

create table task_idempotency_keys (
    tenant_id varchar(64) not null,
    user_id varchar(64) not null,
    client_request_id varchar(160) not null,
    task_id varchar(64) not null,
    created_at timestamptz not null default now(),
    primary key (tenant_id, user_id, client_request_id),
    constraint fk_task_idempotency_task foreign key (tenant_id, task_id) references agent_tasks (tenant_id, task_id) on delete cascade,
    constraint chk_task_idempotency_client_request_id_not_blank check (btrim(client_request_id) <> '')
);

create index idx_task_idempotency_task
on task_idempotency_keys (tenant_id, task_id);

insert into tenants (tenant_id, name, status, metadata)
values ('tenant_local', 'Local Development Tenant', 'ACTIVE', '{"seed":"000003_local_production_loop"}'::jsonb)
on conflict (tenant_id) do update
set name = excluded.name,
    status = excluded.status,
    updated_at = now();

insert into users (tenant_id, user_id, email, display_name, role, status, metadata)
values
    ('tenant_local', 'user_local', 'user.local@stableagent.local', 'Local User', 'member', 'ACTIVE', '{"seed":"000003_local_production_loop"}'::jsonb),
    ('tenant_local', 'admin_local', 'admin.local@stableagent.local', 'Local Admin', 'admin', 'ACTIVE', '{"seed":"000003_local_production_loop"}'::jsonb)
on conflict (tenant_id, user_id) do update
set email = excluded.email,
    display_name = excluded.display_name,
    role = excluded.role,
    status = excluded.status,
    updated_at = now();
