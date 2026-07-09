-- 回滚 000005_admin_console:按依赖倒序删除新对象并还原被修改列。

alter table tenant_tool_policies drop constraint if exists uq_tenant_tool_policies_tool_role;
alter table tenant_tool_policies add constraint uq_tenant_tool_policies_tool unique (tenant_id, tool_name);

alter table tenant_tool_policies drop column if exists timeout_seconds;
alter table tenant_tool_policies drop column if exists argument_rules;
alter table tenant_tool_policies drop column if exists max_calls_per_day;
alter table tenant_tool_policies drop column if exists role;

alter table tenants drop column if exists config;

alter table users drop column if exists last_login_at;

drop table if exists task_templates;
drop table if exists system_settings;
drop table if exists audit_logs;
drop table if exists tools;
