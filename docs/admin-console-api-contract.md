# StableAgent 管理台 API 契约（前后端对齐依据）

本文档是管理台前端（web-ui）与后端（api-service）实现的**唯一对齐依据**。设计背景见 `docs/admin-console-design.md`。

## 0. 通用约定

- 成功响应统一为 `{"data": ...}`，错误响应统一为 `{"error": {"code", "message", "request_id"}}`（见 `internal/transport/http/json.go`）。
- 所有字段一律 snake_case；时间为 RFC3339 字符串。
- 列表响应统一：`{"items": [...], "page": 1, "page_size": 20, "has_more": false, "next_page": null}`；查询参数统一 `page` / `page_size`（上限 100）。
- 鉴权：JWT Bearer，claims 含 `sub`(user_id)、`tenant_id`、`role`。
- 角色取值（小写）：`user`、`approver`、`auditor`、`tenant_admin`、`platform_admin`。历史遗留 `member` 等价于 `user`。
- 数据隔离规则：
  - 非 `platform_admin` 一律限定在 token 的 `tenant_id` 内；
  - `user` 的任务/事件/产物类查询限定 `created_by = sub`；
  - `auditor` 全部只读（写接口返回 403 `FORBIDDEN`）；
  - `platform_admin` 可通过查询参数 `tenant_id` 跨租户过滤。
- 已存在的接口（tasks CRUD、events、SSE、per-task tool-calls/checkpoints/artifacts、approve/reject）**契约不变**，以 `api/openapi.yaml` 和现有 Go handler 为准；本文只定义新增/扩展部分。

## 1. Dashboard

时间范围参数统一：`range=1h|24h|7d`（默认 `24h`），或 `from`/`to`（RFC3339）。`platform_admin` 可加 `tenant_id`。

### GET /api/v1/dashboard/summary
```json
{ "total_tasks": 120, "created_today": 15, "running": 3, "waiting_approval": 2,
  "failed": 4, "completed": 100, "stopped_by_limit": 1, "cancelled": 10,
  "success_rate": 0.92, "avg_duration_seconds": 340 }
```

### GET /api/v1/dashboard/task-stats
按小时/天分桶的时间序列（range=1h 按分钟，24h 按小时，7d 按天）：
```json
{ "buckets": [ { "time": "...", "created": 3, "completed": 2, "failed": 1, "tokens": 12000, "cost_usd": 0.42 } ],
  "status_distribution": [ { "status": "COMPLETED", "count": 100 } ] }
```

### GET /api/v1/dashboard/cost-stats
```json
{ "tokens_today": 500000, "cost_today_usd": 12.5, "avg_cost_per_task_usd": 0.6,
  "tool_calls_total": 240, "high_risk_tool_calls": 12, "approval_blocked": 3 }
```

### GET /api/v1/dashboard/risk-stats
```json
{ "pending_approvals": 2, "rejected_tool_calls": 5, "loop_detected_tasks": 1,
  "token_limit_tasks": 2, "step_limit_tasks": 1,
  "high_risk_trend": [ { "time": "...", "count": 3 } ],
  "risk_distribution": [ { "risk_level": "HIGH", "count": 12 } ] }
```

### GET /api/v1/dashboard/system-health
逐个探测依赖（DB ping、Redis ping、各服务 /healthz、Temporal 连接），单项失败不拖垮整体：
```json
{ "services": [ { "name": "api-service", "status": "healthy|degraded|down", "detail": "" } ],
  "queue_backlog": 0, "active_workers": 1, "checked_at": "..." }
```

## 2. Tasks 扩展

### POST /api/v1/tasks/{task_id}/pause
仅 `RUNNING` 可暂停；返回 `{ "task_id", "status": "PAUSED" }`。owner / tenant_admin / platform_admin 可操作。

### POST /api/v1/tasks/{task_id}/resume-from-checkpoint
Body: `{ "checkpoint_id": "ckpt_..." }`。仅 owner / tenant_admin / platform_admin。校验 checkpoint 属于该任务。返回同 resume：`{ "task_id", "status", "resume_from_checkpoint_id" }`。

### GET /api/v1/tasks/{task_id}/state
返回当前 AgentState：
```json
{ "task_id", "version": 12, "current_plan": "...", "current_step": 3,
  "memory_summary": "...", "constraints": {}, "artifacts": [], "loop_fingerprints": [],
  "state": {}, "updated_at": "..." }
```
`state`（完整 JSON）仅 tenant_admin/platform_admin/auditor 返回；`user` 只返回摘要字段。敏感键（`token`、`secret`、`password`、`api_key`、`authorization`）值一律替换为 `"***"`（递归脱敏，所有返回原始 JSON 的接口通用此规则）。

### 任务列表扩展查询参数（GET /api/v1/tasks）
新增：`goal`（模糊）、`user_id`、`tenant_id`（仅 platform_admin）、`created_from`/`created_to`、`waiting_approval=true`、`stopped_by_limit=true`。

## 3. Approvals

### GET /api/v1/approvals
参数：`status=PENDING|APPROVED|REJECTED|EXPIRED`（默认 PENDING）、`mine=true`（我审批过的）、`task_id`、`tool_name`、`risk_level`、时间范围、分页。
item 字段：`approval_id, task_id, call_id, tool_name, risk_level, approval_reason, requester_id, approver_id, status, note, created_at, decided_at, expires_at`。
兼容别名：`GET /api/v1/approvals/pending` 等价 `status=PENDING`。

### GET /api/v1/approvals/{approval_id}
在列表字段基础上追加：`arguments`（脱敏后的工具参数 JSON）、`task_goal`、`current_step`、`idempotency_key`、`history`（该 call 的全部审批记录数组）。

审批动作沿用现有 `POST /api/v1/tool-calls/{call_id}/approve|reject`，扩展 body：
- approve: `{ "note": "" }`
- reject: `{ "reason": "必填", "terminate_task": false }`
权限：approver / tenant_admin / platform_admin。审批动作必须写 audit_logs。

## 4. Tools

### GET /api/v1/tools
（tenant_admin/platform_admin/auditor 全量；其余角色仅 `enabled=true` 摘要）
item: `tool_name, description, category, default_risk_level, enabled, version, owner, has_side_effect, requires_idempotency_key, timeout_seconds, created_at, updated_at`。

### GET /api/v1/tools/available
供创建任务选择：`[ { "tool_name", "description", "risk_level", "approval_required" } ]`（按当前用户租户策略过滤）。

### GET /api/v1/tools/{tool_name}
详情 = 列表字段 + `params_schema`、`result_schema`、`retry_policy` + 统计：`{ "calls_7d", "error_rate_7d", "avg_duration_ms_7d" }`。

### PATCH /api/v1/tools/{tool_name}
（tenant_admin/platform_admin）可改：`enabled, default_risk_level, timeout_seconds, description, category, params_schema, retry_policy`。写 audit_logs。

## 5. Tool Policies

基于现有 `tenant_tool_policies` 表。

- `GET /api/v1/tool-policies`：参数 `tenant_id`（platform_admin）、`tool_name`、`role`、`enabled`、分页。
- `POST /api/v1/tool-policies`：body 即策略对象：`{ tenant_id, tool_name, role, enabled, approval_required, max_calls_per_day, risk_level_override, argument_rules, timeout_seconds }`；返回含 `policy_id`。
- `GET/PUT/DELETE /api/v1/tool-policies/{policy_id}`（DELETE 为软禁用 `enabled=false`）。
- `POST /api/v1/tool-policies/simulate`：body `{ tenant_id?, user_id?, role, tool_name, arguments }`，返回：
```json
{ "allowed": true, "risk_level": "HIGH", "approval_required": true,
  "matched_policy_id": "...", "deny_reason": null }
```
写接口仅 tenant_admin（本租户）/ platform_admin；全部写 audit_logs。

## 6. Tool Calls（跨任务）

### GET /api/v1/tool-calls
参数：`task_id, call_id, tool_name, status, risk_level, approval_status, user_id, tenant_id, error_code, idempotency_key, created_from, created_to`、分页。
item 同现有 per-task 响应 + `tenant_id, duration_ms, idempotency_key`。

### GET /api/v1/tool-calls/{call_id}
详情：`arguments`（脱敏）、`arguments_hash, idempotency_key, result（脱敏）, error_code, error_message, approvals（数组）, trace_id, duration_ms` 及基础字段。

## 7. Observability

### GET /api/v1/events（Timeline Explorer）
参数：`task_id, event_id, type, tool_name, error_code, trace_id, user_id, tenant_id, created_from, created_to`、分页（按 created_at desc）。
item：`event_id, task_id, type, input_summary, output_summary, metadata, trace_id, created_at`（input/output 截断至 500 字符并脱敏）。

### GET /api/v1/observability/trace/{trace_id}
轻量版：从 agent_events 聚合该 trace 的 span 概览：
```json
{ "trace_id", "task_id", "total_duration_ms", "spans": [
  { "name": "step-3/llm_call", "type": "LLM|TOOL|STEP|CHECKPOINT", "start": "...", "duration_ms": 1200, "status": "ok|error" } ],
  "external_url": "http://tempo.example/trace/{trace_id}" }
```
`external_url` 模板来自系统设置 `observability.trace_url_template`，为空则省略。

### GET /api/v1/observability/metrics
DB 聚合的平台指标快照（参数 `range` 同 dashboard）：
```json
{ "tasks": { "created_total", "completed_total", "failed_total", "cancelled_total",
             "running", "waiting_approval", "duration_p50_s", "duration_p95_s", "duration_p99_s" },
  "llm":   { "calls_total", "errors_total", "latency_p95_ms", "tokens_total", "cost_total_usd",
             "model_distribution": [ { "model", "count" } ] },
  "tools": { "calls_total", "errors_total", "latency_p95_ms",
             "risk_distribution": [ { "risk_level", "count" } ],
             "approval_required_total", "approval_rejected_total" },
  "workers": { "active_workers", "workflow_backlog", "queue_lag": null } }
```
percentile 用 `percentile_cont` 计算；worker 指标取自 Temporal/Redis 可得数据，取不到置 null。

### GET /api/v1/observability/logs
轻量版：由 agent_events + audit_logs 映射为日志行。参数：`task_id, trace_id, tenant_id, level, service, error_code, event_type, created_from, created_to`、分页。
item：`timestamp, level(info|warn|error), service, message, task_id, trace_id, tenant_id, error_code`。
（LOOP_DETECTED/LIMIT_EXCEEDED → warn；TASK_FAILED/TOOL 错误 → error；其余 info。message 必须脱敏。）

## 8. Artifacts（跨任务）

- `GET /api/v1/artifacts`：参数 `task_id, type, name, created_from, created_to`、分页。item：`artifact_id, task_id, name, type, size_bytes, created_by_event_id, created_at`。
- `GET /api/v1/artifacts/{artifact_id}`：详情 + `metadata` + `content`（内联小产物，>1MB 时置 null 并给 `download_url`）。
- `GET /api/v1/artifacts/{artifact_id}/download`：按 Content-Type 输出原始内容。
- `DELETE /api/v1/artifacts/{artifact_id}`：仅 tenant_admin/platform_admin，写 audit_logs。

## 9. Tenants（仅 platform_admin；GET 详情本租户 tenant_admin 可见）

- `GET /api/v1/tenants`：item：`tenant_id, name, status(active|disabled), config, usage: { tasks_today, tokens_today, cost_today_usd }, created_at`。
- `POST /api/v1/tenants`：`{ "name", "config" }`。
- `GET /api/v1/tenants/{tenant_id}`
- `PATCH /api/v1/tenants/{tenant_id}`：可改 `name, status, config`。
`config` JSONB：`{ max_tasks_per_day, max_tokens_per_day, max_cost_per_day, default_max_steps, default_tool_policy, approval_policy, data_retention_days }`。写 audit_logs。

## 10. Users & Roles

- `GET /api/v1/users`（tenant_admin 本租户 / platform_admin 全部）：item：`user_id, email, name, tenant_id, role, status(active|disabled), last_login_at, created_at`。
- `POST /api/v1/users`：邀请 `{ "email", "name", "role", "tenant_id"(platform_admin) }`。
- `PATCH /api/v1/users/{user_id}`：可改 `role, status, name`。tenant_admin 不能设 `platform_admin`，不能改自己角色。写 audit_logs。

## 11. Settings（platform_admin；GET 时 tenant_admin 只读）

- `GET /api/v1/settings`：返回全部分组 `{ "runtime": {...}, "llm": {...}, "tool": {...}, "security": {...}, "retention": {...}, "observability": {...} }`。
- `PUT /api/v1/settings/{section}`：整组覆盖写入，section 为上述 key。写 audit_logs。
存储：`system_settings(section text pk, value jsonb, updated_by, updated_at)`；各组字段按设计稿第 16 节，未设置时返回代码默认值。secret 类字段读取时脱敏为 `***`。

## 12. Audit Logs（auditor/tenant_admin/platform_admin）

### GET /api/v1/audit-logs
参数：`actor_id, action, resource_type, resource_id, tenant_id, created_from, created_to`、分页。
item：`audit_id, actor_id, tenant_id, action, resource_type, resource_id, before, after, ip, user_agent, created_at`。

必须落审计的动作（action 命名）：`task.create / task.cancel / task.pause / task.resume / task.resume_from_checkpoint / approval.approve / approval.reject / tool.update / tool_policy.create / tool_policy.update / tool_policy.delete / tenant.create / tenant.update / user.invite / user.update / artifact.delete / settings.update`。

## 13. Task Templates

- `GET /api/v1/task-templates`：本人 + 租户共享模板。item：`template_id, name, payload(创建任务请求体), shared, created_by, created_at`。
- `POST /api/v1/task-templates`：`{ "name", "payload", "shared": false }`。
- `DELETE /api/v1/task-templates/{template_id}`：创建者或管理员。

## 14. 新增数据库对象（migration 000005）

- `tools`（工具注册表，含 seed：现有 tool-gateway 内置工具全部插入）
- `audit_logs`
- `system_settings`
- `task_templates`
- `users` 表补列（如缺）：`status, last_login_at`
- `tenants` 表补列（如缺）：`status, config jsonb`
- `tenant_tool_policies` 补列（如缺）：`role, max_calls_per_day, argument_rules jsonb, timeout_seconds`

## 15. 错误码

沿用 `pkg/errors`：`INVALID_ARGUMENT`(400)、`UNAUTHORIZED`(401)、`FORBIDDEN`(403)、`NOT_FOUND`(404)、`CONFLICT`(409)、`INTERNAL`(500)。权限不足一律 `FORBIDDEN`，且不得泄露资源是否存在（跨租户访问返回 404）。
