-- 第 2 周核心数据模型：任务、状态、事件、工具调用、checkpoint、审批、artifact、outbox 与租户基础表。

create type tenant_status as enum ('ACTIVE', 'SUSPENDED', 'DELETED');
create type user_status as enum ('ACTIVE', 'DISABLED', 'DELETED');
create type task_status as enum (
    'QUEUED',
    'RUNNING',
    'WAITING_APPROVAL',
    'PAUSED',
    'CANCELING',
    'CANCELED',
    'SUCCEEDED',
    'FAILED'
);
create type tool_call_status as enum (
    'PENDING',
    'RUNNING',
    'WAITING_APPROVAL',
    'SUCCEEDED',
    'FAILED',
    'CANCELED'
);
create type approval_status as enum (
    'PENDING',
    'APPROVED',
    'REJECTED',
    'EXPIRED',
    'CANCELED'
);
create type risk_level as enum ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL');
create type outbox_status as enum ('PENDING', 'PROCESSING', 'SENT', 'FAILED', 'DEAD');
create type artifact_type as enum ('TEXT', 'JSON', 'FILE', 'IMAGE', 'REPORT', 'OTHER');
create type artifact_status as enum ('AVAILABLE', 'DELETED');
create type tool_policy_effect as enum ('ALLOW', 'DENY', 'REQUIRE_APPROVAL');

create table tenants (
    tenant_id varchar(64) primary key,
    name varchar(128) not null,
    status tenant_status not null default 'ACTIVE',
    metadata jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint chk_tenants_id_not_blank check (btrim(tenant_id) <> ''),
    constraint chk_tenants_name_not_blank check (btrim(name) <> ''),
    constraint chk_tenants_metadata_object check (jsonb_typeof(metadata) = 'object')
);

create table users (
    tenant_id varchar(64) not null,
    user_id varchar(64) not null,
    email varchar(320) not null,
    display_name varchar(128),
    role varchar(64) not null default 'member',
    status user_status not null default 'ACTIVE',
    metadata jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (tenant_id, user_id),
    constraint fk_users_tenant foreign key (tenant_id) references tenants (tenant_id) on delete restrict,
    constraint uq_users_tenant_email unique (tenant_id, email),
    constraint chk_users_id_not_blank check (btrim(user_id) <> ''),
    constraint chk_users_email_not_blank check (btrim(email) <> ''),
    constraint chk_users_role_not_blank check (btrim(role) <> ''),
    constraint chk_users_metadata_object check (jsonb_typeof(metadata) = 'object')
);

create index idx_users_tenant_status
on users (tenant_id, status, created_at desc);

create table agent_tasks (
    task_id varchar(64) primary key,
    tenant_id varchar(64) not null,
    user_id varchar(64) not null,
    goal text not null,
    status task_status not null default 'QUEUED',
    budget jsonb not null default '{}'::jsonb,
    budget_usage jsonb not null default '{}'::jsonb,
    workflow_id varchar(128),
    trace_id varchar(128),
    last_error_code varchar(64),
    last_error_message text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint uq_agent_tasks_tenant_task unique (tenant_id, task_id),
    constraint fk_agent_tasks_user foreign key (tenant_id, user_id) references users (tenant_id, user_id) on delete restrict,
    constraint chk_agent_tasks_id_not_blank check (btrim(task_id) <> ''),
    constraint chk_agent_tasks_goal_not_blank check (btrim(goal) <> ''),
    constraint chk_agent_tasks_budget_object check (jsonb_typeof(budget) = 'object'),
    constraint chk_agent_tasks_budget_usage_object check (jsonb_typeof(budget_usage) = 'object')
);

create index idx_agent_tasks_tenant_user_status
on agent_tasks (tenant_id, user_id, status, created_at desc);

create index idx_agent_tasks_status_updated
on agent_tasks (status, updated_at desc);

create index idx_agent_tasks_tenant_status_updated
on agent_tasks (tenant_id, status, updated_at desc);

create table agent_states (
    task_id varchar(64) primary key,
    tenant_id varchar(64) not null,
    current_plan jsonb not null default '{}'::jsonb,
    current_step int not null default 0,
    memory_summary text,
    constraints jsonb not null default '{}'::jsonb,
    artifacts jsonb not null default '[]'::jsonb,
    loop_fingerprints jsonb not null default '[]'::jsonb,
    version int not null default 1,
    updated_at timestamptz not null default now(),
    constraint fk_agent_states_task foreign key (tenant_id, task_id) references agent_tasks (tenant_id, task_id) on delete cascade,
    constraint chk_agent_states_current_step check (current_step >= 0),
    constraint chk_agent_states_version check (version > 0),
    constraint chk_agent_states_current_plan_object check (jsonb_typeof(current_plan) = 'object'),
    constraint chk_agent_states_constraints_object check (jsonb_typeof(constraints) = 'object'),
    constraint chk_agent_states_artifacts_array check (jsonb_typeof(artifacts) = 'array'),
    constraint chk_agent_states_loop_fingerprints_array check (jsonb_typeof(loop_fingerprints) = 'array')
);

create index idx_agent_states_tenant
on agent_states (tenant_id);

create table agent_events (
    event_id bigserial primary key,
    task_id varchar(64) not null,
    tenant_id varchar(64) not null,
    type varchar(64) not null,
    input jsonb,
    output jsonb,
    metadata jsonb,
    trace_id varchar(128),
    span_id varchar(128),
    created_at timestamptz not null default now(),
    constraint fk_agent_events_task foreign key (tenant_id, task_id) references agent_tasks (tenant_id, task_id) on delete cascade,
    constraint chk_agent_events_type_not_blank check (btrim(type) <> '')
);

create index idx_agent_events_task_event
on agent_events (task_id, event_id);

create index idx_agent_events_tenant_task_event
on agent_events (tenant_id, task_id, event_id);

create index idx_agent_events_tenant_type_time
on agent_events (tenant_id, type, created_at desc);

create table artifacts (
    artifact_id varchar(64) primary key,
    tenant_id varchar(64) not null,
    task_id varchar(64) not null,
    user_id varchar(64) not null,
    type artifact_type not null,
    name varchar(256) not null,
    media_type varchar(128),
    storage_backend varchar(32) not null default 'MINIO',
    storage_key text,
    content_json jsonb,
    size_bytes bigint not null default 0,
    checksum_sha256 varchar(64),
    metadata jsonb not null default '{}'::jsonb,
    status artifact_status not null default 'AVAILABLE',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint fk_artifacts_task foreign key (tenant_id, task_id) references agent_tasks (tenant_id, task_id) on delete cascade,
    constraint fk_artifacts_user foreign key (tenant_id, user_id) references users (tenant_id, user_id) on delete restrict,
    constraint chk_artifacts_id_not_blank check (btrim(artifact_id) <> ''),
    constraint chk_artifacts_name_not_blank check (btrim(name) <> ''),
    constraint chk_artifacts_storage_backend_not_blank check (btrim(storage_backend) <> ''),
    constraint chk_artifacts_payload_present check (storage_key is not null or content_json is not null),
    constraint chk_artifacts_size_bytes check (size_bytes >= 0),
    constraint chk_artifacts_metadata_object check (jsonb_typeof(metadata) = 'object')
);

create index idx_artifacts_tenant_task_created
on artifacts (tenant_id, task_id, created_at desc);

create index idx_artifacts_tenant_status_created
on artifacts (tenant_id, status, created_at desc);

create table tool_calls (
    call_id varchar(64) primary key,
    task_id varchar(64) not null,
    tenant_id varchar(64) not null,
    user_id varchar(64) not null,
    tool_name varchar(128) not null,
    arguments jsonb not null,
    arguments_hash varchar(128) not null,
    idempotency_key varchar(160) not null,
    status tool_call_status not null default 'PENDING',
    result jsonb,
    result_artifact_id varchar(64),
    risk_level risk_level not null,
    approval_status approval_status not null,
    error_code varchar(64),
    error_message text,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint uq_tool_calls_tenant_call unique (tenant_id, call_id),
    constraint uq_tool_calls_tenant_idempotency unique (tenant_id, idempotency_key),
    constraint fk_tool_calls_task foreign key (tenant_id, task_id) references agent_tasks (tenant_id, task_id) on delete cascade,
    constraint fk_tool_calls_user foreign key (tenant_id, user_id) references users (tenant_id, user_id) on delete restrict,
    constraint fk_tool_calls_result_artifact foreign key (result_artifact_id) references artifacts (artifact_id) on delete set null,
    constraint chk_tool_calls_id_not_blank check (btrim(call_id) <> ''),
    constraint chk_tool_calls_tool_name_not_blank check (btrim(tool_name) <> ''),
    constraint chk_tool_calls_arguments_hash_not_blank check (btrim(arguments_hash) <> ''),
    constraint chk_tool_calls_idempotency_key_not_blank check (btrim(idempotency_key) <> ''),
    constraint chk_tool_calls_arguments_object check (jsonb_typeof(arguments) = 'object')
);

create index idx_tool_calls_task_status
on tool_calls (task_id, status);

create index idx_tool_calls_tenant_task_status
on tool_calls (tenant_id, task_id, status);

create index idx_tool_calls_tenant_tool_time
on tool_calls (tenant_id, tool_name, created_at desc);

create table checkpoints (
    checkpoint_id varchar(64) primary key,
    task_id varchar(64) not null,
    tenant_id varchar(64) not null,
    state_version int not null,
    event_offset bigint not null,
    state_snapshot jsonb not null,
    reason varchar(64) not null,
    trace_id varchar(128),
    created_at timestamptz not null default now(),
    constraint fk_checkpoints_task foreign key (tenant_id, task_id) references agent_tasks (tenant_id, task_id) on delete cascade,
    constraint chk_checkpoints_id_not_blank check (btrim(checkpoint_id) <> ''),
    constraint chk_checkpoints_state_version check (state_version > 0),
    constraint chk_checkpoints_event_offset check (event_offset >= 0),
    constraint chk_checkpoints_reason_not_blank check (btrim(reason) <> ''),
    constraint chk_checkpoints_state_snapshot_object check (jsonb_typeof(state_snapshot) = 'object')
);

create index idx_checkpoints_task_created
on checkpoints (task_id, created_at desc);

create index idx_checkpoints_tenant_task_created
on checkpoints (tenant_id, task_id, created_at desc);

create table approvals (
    approval_id varchar(64) primary key,
    task_id varchar(64) not null,
    tenant_id varchar(64) not null,
    call_id varchar(64) not null,
    approver_id varchar(64),
    status approval_status not null,
    risk_level risk_level not null,
    approval_reason text,
    comment text,
    expires_at timestamptz,
    decided_at timestamptz,
    created_at timestamptz not null default now(),
    constraint uq_approvals_call unique (call_id),
    constraint fk_approvals_task foreign key (tenant_id, task_id) references agent_tasks (tenant_id, task_id) on delete cascade,
    constraint fk_approvals_call foreign key (tenant_id, call_id) references tool_calls (tenant_id, call_id) on delete cascade,
    constraint chk_approvals_id_not_blank check (btrim(approval_id) <> '')
);

create index idx_approvals_tenant_status
on approvals (tenant_id, status, created_at desc);

create index idx_approvals_approver_status
on approvals (approver_id, status, created_at desc);

create table task_outbox (
    outbox_id bigserial primary key,
    tenant_id varchar(64) not null,
    aggregate_type varchar(64) not null,
    aggregate_id varchar(64) not null,
    event_type varchar(64) not null,
    payload jsonb not null,
    status outbox_status not null default 'PENDING',
    retry_count int not null default 0,
    next_retry_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint fk_task_outbox_tenant foreign key (tenant_id) references tenants (tenant_id) on delete restrict,
    constraint chk_task_outbox_aggregate_type_not_blank check (btrim(aggregate_type) <> ''),
    constraint chk_task_outbox_aggregate_id_not_blank check (btrim(aggregate_id) <> ''),
    constraint chk_task_outbox_event_type_not_blank check (btrim(event_type) <> ''),
    constraint chk_task_outbox_payload_object check (jsonb_typeof(payload) = 'object'),
    constraint chk_task_outbox_retry_count check (retry_count >= 0)
);

create index idx_task_outbox_status_retry
on task_outbox (status, next_retry_at);

create index idx_task_outbox_tenant_created
on task_outbox (tenant_id, created_at desc);

create table tenant_tool_policies (
    policy_id varchar(64) primary key,
    tenant_id varchar(64) not null,
    tool_name varchar(128) not null,
    effect tool_policy_effect not null default 'ALLOW',
    max_risk_level risk_level not null default 'LOW',
    require_approval boolean not null default false,
    config jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint fk_tenant_tool_policies_tenant foreign key (tenant_id) references tenants (tenant_id) on delete cascade,
    constraint uq_tenant_tool_policies_tool unique (tenant_id, tool_name),
    constraint chk_tenant_tool_policies_id_not_blank check (btrim(policy_id) <> ''),
    constraint chk_tenant_tool_policies_tool_name_not_blank check (btrim(tool_name) <> ''),
    constraint chk_tenant_tool_policies_config_object check (jsonb_typeof(config) = 'object')
);

create index idx_tenant_tool_policies_tenant_effect
on tenant_tool_policies (tenant_id, effect);
