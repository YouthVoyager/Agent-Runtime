-- 设计方案 §9.1 状态枚举补齐:预算/循环超限停止与可重试失败。
-- psql -f 按语句自动提交,ALTER TYPE ADD VALUE 无需事务包装。
alter type task_status add value if not exists 'STOPPED_BY_LIMIT';
alter type task_status add value if not exists 'RETRYABLE_FAILED';
