# StableAgent 本地生产闭环技术文档

## 1. 实现目标

本次实现把 StableAgent 从“任务 API + timeline 原型”推进到本地 Docker Compose 可运行的端到端闭环。闭环覆盖任务创建、outbox 派发、本地 workflow 执行、Mock LLM、Tool Gateway、人工审批、checkpoint、cancel/resume、artifact、SSE timeline 和 Prometheus 格式 `/metrics`。

本轮不接真实 LLM Provider。LLM Gateway 使用可替换 Mock Provider，Tool Gateway 使用本地 Mock Executor，接口边界按生产接入方式保留。

## 2. 状态兼容

| 设计方案语义 | 当前数据库状态 |
| --- | --- |
| 等待执行 | `QUEUED` |
| 执行中 | `RUNNING` |
| 等待审批 | `WAITING_APPROVAL` |
| 暂停 | `PAUSED` |
| 请求取消 | `CANCELING` |
| 已取消 | `CANCELED` |
| 已完成 | `SUCCEEDED` |
| 失败 | `FAILED` |

设计方案中使用 `CANCEL_REQUESTED/CANCELLED/COMPLETED` 等命名；为了不破坏已有 sqlc 生成代码和迁移历史，本轮沿用当前状态枚举。

## 3. 核心链路

### 3.1 创建任务

`POST /api/v1/tasks` 从 JWT 读取 `tenant_id/user_id/role`，不信任请求体中的租户字段。创建事务内写入 `agent_tasks`、`agent_states`、`TASK_CREATED` event、`START_WORKFLOW` outbox 和可选 `task_idempotency_keys`。

`client_request_id` 使用独立表实现幂等，避免给 `agent_tasks` 增加列导致现有 sqlc `select *` 扫描失配。

### 3.2 Worker 本地 workflow

`runtime-worker` 启动后持续扫描 `task_outbox`，把 `task_id` 写为 `workflow_id`，再推进 `QUEUED/CANCELING` 任务。执行过程中会写 `BEFORE_LLM`、`AFTER_LLM`、`AFTER_TOOL` checkpoint，调用 `LLM Gateway /llm/v1/chat` 和 `Tool Gateway /tools/v1/calls`，最终把任务落到 `SUCCEEDED`、`WAITING_APPROVAL`、`CANCELED` 或 `FAILED`。

本地 workflow 没有引入 Temporal SDK；`workflow_id=task_id` 与 outbox 补偿语义已固定，后续可把本地执行器替换成 Temporal Workflow/Activity，而 API、DB 和 Gateway 边界不需要重做。

### 3.3 Tool Gateway

| 工具 | 风险 | 行为 |
| --- | --- | --- |
| `read_document` | `LOW` | 返回本地设计文档摘要 |
| `write_artifact` | `MEDIUM` | 写入 `artifacts` 表，当前小内容存 PostgreSQL |
| `send_external_message` | `HIGH` | 创建 `approvals`，等待人工审批 |
| `dangerous_admin_action` | `CRITICAL` | 本地闭环默认拒绝 |

工具调用幂等键由 `task_id + step + tool_name + arguments_hash` 生成。高风险工具审批通过后，API 会把 `tool_calls.status` 重置为 `PENDING`、`approval_status` 置为 `APPROVED`，并写入 `START_WORKFLOW` outbox。

### 3.4 Cancel 和 Resume

Cancel 会写 Redis cancel flag，并将任务置为 `CANCELING`。Worker 扫描到后在安全边界置为 `CANCELED`，清理 Redis flag，并写 `TASK_CANCELLED`。

Resume 会校验最近 checkpoint，重新把任务置为 `QUEUED` 并写入 `START_WORKFLOW` outbox。副作用工具通过 `tool_calls.idempotency_key` 避免重复执行。

## 4. 新增 API

| API | 说明 |
| --- | --- |
| `POST /api/v1/tasks/{task_id}/cancel` | 请求取消任务 |
| `POST /api/v1/tasks/{task_id}/resume` | 从最近 checkpoint 恢复 |
| `GET /api/v1/tasks/{task_id}/tool-calls` | 查询工具调用 |
| `GET /api/v1/tasks/{task_id}/checkpoints` | 查询 checkpoint |
| `GET /api/v1/tasks/{task_id}/artifacts` | 查询 artifact |
| `POST /api/v1/tool-calls/{call_id}/approve` | 审批通过工具调用 |
| `POST /api/v1/tool-calls/{call_id}/reject` | 审批拒绝工具调用 |
| `POST /llm/v1/chat` | 内部 LLM Gateway Mock 接口 |
| `POST /tools/v1/calls` | 内部 Tool Gateway 调用接口 |
| `GET /metrics` | Prometheus 文本格式基础指标 |

## 5. 本地验证

```bash
make docker-up
make migrate-up
make health
make jwt
make e2e-local
```

`make e2e-local` 覆盖普通任务完成、高风险审批通过、等待审批任务取消、CRITICAL 工具失败和 resume 后重复失败但不重复副作用的场景。

## 6. 生产扩展点

1. Temporal：当前本地 workflow 可替换为 `AgentTaskWorkflow`，outbox 的 `START_WORKFLOW` 已是稳定入口。
2. LLM Provider：`/llm/v1/chat` 当前为 Mock，实现真实 Provider 时保持 token usage、model、prompt version。
3. Object Store：当前小 artifact 存 PostgreSQL，大对象可扩展到 MinIO/S3 并回填 `storage_key`。
4. Metrics：当前 `/metrics` 提供基础 up/uptime，后续可接入任务成功率、审批等待时间、tool latency。
5. OpenTelemetry：当前 trace 字段随 task/event 传递，后续可在 HTTP middleware 和 Worker step 中创建真实 span。
