-- 管理台补齐:工具注册表、审计日志、系统设置、任务模板,以及现有表补列。
-- 对齐 docs/admin-console-api-contract.md 第 14 节。

-- =========================================================================
-- 1. tools 工具注册表:管理台工具管理页面的数据来源,seed 写入本地闭环内置工具。
-- =========================================================================
create table tools (
    tool_name varchar(128) primary key,
    description text not null default '',
    category varchar(64) not null default 'general',
    default_risk_level risk_level not null default 'LOW',
    enabled boolean not null default true,
    version varchar(32) not null default 'v1',
    owner varchar(128) not null default 'platform',
    has_side_effect boolean not null default false,
    requires_idempotency_key boolean not null default true,
    timeout_seconds int not null default 30,
    params_schema jsonb not null default '{}'::jsonb,
    result_schema jsonb not null default '{}'::jsonb,
    retry_policy jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint chk_tools_name_not_blank check (btrim(tool_name) <> ''),
    constraint chk_tools_timeout_positive check (timeout_seconds > 0),
    constraint chk_tools_params_schema_object check (jsonb_typeof(params_schema) = 'object'),
    constraint chk_tools_result_schema_object check (jsonb_typeof(result_schema) = 'object'),
    constraint chk_tools_retry_policy_object check (jsonb_typeof(retry_policy) = 'object')
);

-- 与 internal/domain/toolcall.Registry() 保持一致的本地闭环内置工具目录。
insert into tools (tool_name, description, category, default_risk_level, enabled, version, owner, has_side_effect, requires_idempotency_key, timeout_seconds, params_schema)
values
    ('read_document', '读取本地任务上下文中的文档摘要，不产生外部副作用', 'document', 'LOW', true, 'v1', 'platform', false, true, 30,
     '{"type":"object","properties":{"path":{"type":"string","description":"文档路径"},"goal":{"type":"string","description":"读取目的"}},"required":["path"]}'::jsonb),
    ('write_artifact', '写入任务产物，小内容保存到 PostgreSQL artifact 表', 'artifact', 'MEDIUM', true, 'v1', 'platform', true, true, 30,
     '{"type":"object","properties":{"name":{"type":"string","description":"产物名称"},"summary":{"type":"string","description":"产物摘要"},"goal":{"type":"string","description":"关联任务目标"}},"required":["name"]}'::jsonb),
    ('send_external_message', '模拟外部可见消息发送，必须人工审批后执行', 'messaging', 'HIGH', true, 'v1', 'platform', true, true, 30,
     '{"type":"object","properties":{"recipient":{"type":"string","description":"接收人地址"},"subject":{"type":"string","description":"消息主题"},"body":{"type":"string","description":"消息正文"}},"required":["recipient","subject"]}'::jsonb),
    ('dangerous_admin_action', '模拟危险管理操作，本地闭环默认禁止执行', 'admin', 'CRITICAL', true, 'v1', 'platform', true, true, 30,
     '{"type":"object","properties":{"action":{"type":"string","description":"管理操作名称"},"reason":{"type":"string","description":"操作原因"}},"required":["action"]}'::jsonb)
on conflict (tool_name) do nothing;

-- =========================================================================
-- 2. audit_logs 审计日志:记录管理台全部敏感写操作。
-- =========================================================================
create table audit_logs (
    audit_id varchar(64) primary key,
    actor_id varchar(64) not null,
    tenant_id varchar(64),
    action varchar(128) not null,
    resource_type varchar(64) not null,
    resource_id varchar(128),
    before jsonb,
    after jsonb,
    ip varchar(64),
    user_agent text,
    created_at timestamptz not null default now(),
    constraint chk_audit_logs_actor_not_blank check (btrim(actor_id) <> ''),
    constraint chk_audit_logs_action_not_blank check (btrim(action) <> ''),
    constraint chk_audit_logs_resource_type_not_blank check (btrim(resource_type) <> '')
);

create index idx_audit_logs_tenant_created on audit_logs (tenant_id, created_at desc);
create index idx_audit_logs_actor_created on audit_logs (actor_id, created_at desc);
create index idx_audit_logs_action_created on audit_logs (action, created_at desc);
create index idx_audit_logs_resource on audit_logs (resource_type, resource_id);

-- =========================================================================
-- 3. system_settings 系统设置:按 section 存整组 JSON 配置。
-- =========================================================================
create table system_settings (
    section varchar(64) primary key,
    value jsonb not null default '{}'::jsonb,
    updated_by varchar(64),
    updated_at timestamptz not null default now(),
    constraint chk_system_settings_section_not_blank check (btrim(section) <> ''),
    constraint chk_system_settings_value_object check (jsonb_typeof(value) = 'object')
);

-- =========================================================================
-- 4. task_templates 任务模板:本人 + 租户共享模板。
-- =========================================================================
create table task_templates (
    template_id varchar(64) primary key,
    tenant_id varchar(64) not null,
    name varchar(256) not null,
    payload jsonb not null,
    shared boolean not null default false,
    created_by varchar(64) not null,
    created_at timestamptz not null default now(),
    constraint fk_task_templates_tenant foreign key (tenant_id) references tenants (tenant_id) on delete cascade,
    constraint chk_task_templates_name_not_blank check (btrim(name) <> ''),
    constraint chk_task_templates_created_by_not_blank check (btrim(created_by) <> ''),
    constraint chk_task_templates_payload_object check (jsonb_typeof(payload) = 'object')
);

create index idx_task_templates_tenant_created
on task_templates (tenant_id, created_at desc);

create index idx_task_templates_tenant_creator
on task_templates (tenant_id, created_by, created_at desc);

-- =========================================================================
-- 5. 现有表补列(设计契约第 14 节)。
-- =========================================================================
alter table users add column if not exists last_login_at timestamptz;

alter table tenants add column if not exists config jsonb not null default '{}'::jsonb;

alter table tenant_tool_policies add column if not exists role varchar(64) not null default '';
alter table tenant_tool_policies add column if not exists max_calls_per_day int;
alter table tenant_tool_policies add column if not exists argument_rules jsonb not null default '{}'::jsonb;
alter table tenant_tool_policies add column if not exists timeout_seconds int;

-- 策略需要按 tenant + tool + role 维度区分(role 空串表示对全部角色生效),
-- 原先 (tenant_id, tool_name) 唯一约束在补充 role 列后需要放宽。
alter table tenant_tool_policies drop constraint if exists uq_tenant_tool_policies_tool;
alter table tenant_tool_policies add constraint uq_tenant_tool_policies_tool_role unique (tenant_id, tool_name, role);
