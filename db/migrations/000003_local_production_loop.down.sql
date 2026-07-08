-- 回滚本地生产闭环补充对象。

drop table if exists task_idempotency_keys;

delete from users
where tenant_id = 'tenant_local'
  and user_id in ('user_local', 'admin_local');

delete from tenants
where tenant_id = 'tenant_local';
