-- 回滚第 2 周核心数据模型。

drop table if exists tenant_tool_policies;
drop table if exists task_outbox;
drop table if exists approvals;
drop table if exists checkpoints;
drop table if exists tool_calls;
drop table if exists artifacts;
drop table if exists agent_events;
drop table if exists agent_states;
drop table if exists agent_tasks;
drop table if exists users;
drop table if exists tenants;

drop type if exists tool_policy_effect;
drop type if exists artifact_status;
drop type if exists artifact_type;
drop type if exists outbox_status;
drop type if exists risk_level;
drop type if exists approval_status;
drop type if exists tool_call_status;
drop type if exists task_status;
drop type if exists user_status;
drop type if exists tenant_status;
