# 第 4 周 AgentEvent 与 Event Timeline 技术文档

## 1. 本周目标

本周实现 StableAgent 的事件记录、timeline 查询、SSE 实时推送和前端 timeline 原型。实现遵循 `设计方案.md` 中的 AgentEvent、timeline、SSE 和可观测 trace 统一设计：

1. `task_id` 作为业务主线。
2. `trace_id` 作为技术链路主线。
3. `event_id` 作为 timeline 主线。
4. `agent_events` 只追加，不修改。
5. event 写入数据库成功后再推送。
6. 推送失败不影响事件持久化和任务执行。

## 2. 代码结构

| 路径 | 作用 |
| --- | --- |
| `internal/domain/event/event.go` | AgentEvent 类型枚举、metadata 结构和 JSON 默认值规范 |
| `internal/usecase/eventapi/service.go` | EventService，负责事件追加、权限校验、timeline 查询和推送触发 |
| `internal/usecase/eventapi/hub.go` | 单实例内存推送 hub，按 `tenant_id + task_id` 管理 SSE 订阅 |
| `internal/infra/redis/event_bus.go` | Redis Pub/Sub 跨实例事件广播 |
| `internal/app/api/event_handler.go` | timeline REST API 和 SSE endpoint |
| `internal/app/api/static_handler.go` | Go 直接托管 React build 静态文件 |
| `web-ui/src/app.js` | React timeline 前端原型 |
| `web-ui/scripts/build.mjs` | 无在线依赖的静态文件 build 脚本 |
| `web-ui/dist` | build 后由 API Service 直接托管的静态文件 |

## 3. AgentEvent 类型设计

事件类型定义在 `internal/domain/event/event.go`。首批事件类型包括：

| 类型 | 含义 |
| --- | --- |
| `TASK_CREATED` | 任务创建成功 |
| `TASK_STARTED` | 任务开始执行 |
| `TASK_COMPLETED` | 任务完成 |
| `TASK_FAILED` | 任务失败 |
| `TASK_CANCELLED` | 任务取消 |
| `TASK_RESUMED` | 任务恢复 |
| `STEP_STARTED` | Agent step 开始 |
| `STEP_COMPLETED` | Agent step 完成 |
| `STEP_FAILED` | Agent step 失败 |
| `LLM_CALL_STARTED` | LLM 调用开始 |
| `LLM_CALL_COMPLETED` | LLM 调用完成 |
| `TOOL_CALL_STARTED` | 工具调用开始 |
| `TOOL_CALL_COMPLETED` | 工具调用完成 |
| `TOOL_APPROVAL_REQUIRED` | 高风险工具需要人工审批 |

`EventService.Append` 会校验事件类型白名单，未知类型会返回 `INVALID_ARGUMENT`，避免数据库中出现前端和审计系统无法解释的事件。

## 4. Metadata 统一结构

metadata 使用 JSON object，空值统一存储为 `{}`。标准字段如下：

| 字段 | 说明 |
| --- | --- |
| `user_id` | 触发事件的用户 |
| `request_id` | HTTP 请求 ID |
| `source` | 事件来源，例如 `api-service`、`runtime-worker` |
| `step_id` | Agent step ID |
| `workflow_id` | Workflow ID |
| `attempt` | 重试次数或执行尝试次数 |
| `extra` | 扩展字段 |

创建任务时，`TASK_CREATED` 事件写入：

```json
{
  "user_id": "user_001",
  "request_id": "demo-create-task",
  "source": "api-service"
}
```

## 5. 写入流程

创建任务仍保持第 3 周的同事务闭环：

1. 写入 `agent_tasks`。
2. 写入 `agent_states`。
3. 写入 `agent_events`，事件类型为 `TASK_CREATED`。
4. 写入 `task_outbox`，事件类型为 `START_WORKFLOW`。
5. 提交事务。
6. 事务提交成功后推送 `TASK_CREATED` 到本地 SSE hub 和 Redis Pub/Sub。

这样可以保证客户端收到的 `event_id` 已经能从数据库查询到，不会出现“推送先到、补拉查不到”的问题。

后续 worker、tool gateway 或 llm gateway 写 AgentEvent 时，应统一调用 `EventService.Append`，不要直接 update/delete `agent_events`。当前代码没有提供任何事件更新或删除接口，符合 append-only 要求。

## 6. Timeline 查询 API

接口：

```http
GET /api/v1/tasks/{task_id}/events?after_event_id=0&limit=100&type=STEP_STARTED
Authorization: Bearer <jwt>
```

参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `after_event_id` | `0` | 只返回 `event_id > after_event_id` 的事件 |
| `limit` | `100` | 单次返回数量，最大 `200` |
| `type` | 空 | 可选事件类型过滤 |

查询始终先读取任务并执行权限校验：

1. SQL 带 `tenant_id + task_id`。
2. `member` 只能看自己创建的任务。
3. `admin/owner` 可以看租户内任务。
4. 结果按 `event_id asc` 返回，保证 timeline 顺序稳定。

响应示例：

```json
{
  "data": {
    "items": [
      {
        "event_id": 1,
        "task_id": "task_001",
        "type": "TASK_CREATED",
        "input": {
          "goal": "分析需求文档并生成生产级 Go 技术方案"
        },
        "output": null,
        "metadata": {
          "user_id": "user_001",
          "request_id": "demo-create-task",
          "source": "api-service"
        },
        "trace_id": "trace_001",
        "span_id": "span_001",
        "created_at": "2026-07-07T10:00:00Z"
      }
    ],
    "next_after_event_id": 1,
    "has_more": false
  }
}
```

## 7. SSE 实时推送

接口：

```http
GET /api/v1/tasks/{task_id}/events/stream
Authorization: Bearer <jwt>
Last-Event-ID: 12
```

服务端行为：

1. 校验 JWT 和任务读取权限。
2. 先订阅本地 hub。
3. 再按 `Last-Event-ID` 从数据库补发历史事件。
4. 进入实时事件循环。
5. 每 15 秒发送一次 `: ping` 心跳。
6. 客户端断开时自动释放订阅。

SSE 事件格式：

```text
id: 13
event: agent_event
data: {"event_id":13,"task_id":"task_001","type":"STEP_COMPLETED"}
```

### 7.1 Last-Event-ID

断线重连时，客户端把最后收到的 `event_id` 作为 `Last-Event-ID` 发送。服务端补发所有 `event_id > Last-Event-ID` 的事件，然后继续实时推送。

前端因为需要携带 `Authorization` header，没有使用浏览器原生 `EventSource`，而是使用 `fetch` 读取 `text/event-stream`，这样可以满足现有 JWT 鉴权设计。

### 7.2 慢客户端保护

`internal/usecase/eventapi/hub.go` 为每个订阅者创建有界 channel，默认 buffer 为 `64`。发布事件时使用非阻塞写入：

1. channel 有空间：写入事件。
2. channel 已满：关闭该订阅者。

慢客户端被断开后，可以通过 `Last-Event-ID` 重连并从数据库补拉，不会阻塞事件写入和其他 SSE 客户端。

## 8. Redis Pub/Sub 多实例推送

单实例内，事件写入成功后直接推送到本地 hub。多实例场景下，只靠本地 hub 会出现“事件写在实例 B，但 SSE 客户端连接实例 A”的问题。

本周接入 Redis Pub/Sub：

1. API Service 启动时创建 Redis EventBus。
2. 每个 API 实例订阅 `stableagent:events`。
3. EventService 写 DB 成功后：
   - 推送到本地 hub。
   - 发布到 Redis channel。
4. 其他 API 实例收到 Redis 消息后，推送到本实例本地 hub。

Redis 发布失败只写告警，不影响数据库事件写入。客户端仍可通过重连和 timeline 查询补拉事件。

## 9. React Timeline 前端

前端目录为 `web-ui`。构建命令：

```bash
npm run build
```

构建产物：

```text
web-ui/dist/index.html
web-ui/dist/assets/app.js
web-ui/dist/assets/styles.css
```

API Service 默认从 `web-ui/dist` 托管静态文件，也可以通过环境变量覆盖：

```bash
API_SERVICE_WEB_STATIC_DIR=web-ui/dist
```

前端能力：

1. 输入 JWT。
2. 输入已有 `task_id` 查询 timeline。
3. 创建新任务后自动切换到新任务 timeline。
4. 展示 `TASK_CREATED`、`STEP_STARTED`、`STEP_COMPLETED`。
5. 初次加载使用 timeline REST API。
6. 实时更新使用 SSE。
7. 断线后用最后收到的 `event_id` 自动重连。

受当前执行环境限制，在线 `npm install` 被权限额度拒绝，本实现采用无在线构建依赖方案。React 在浏览器运行时通过 ESM CDN 加载，Go 仍然直接托管 build 后的静态文件。

## 10. 验证方式

已执行：

```bash
go test ./...
npm run build
node --check web-ui/src/app.js
```

建议本地联调：

1. 启动依赖：

```bash
docker compose -f deploy/docker-compose.yml up -d
```

2. 运行迁移：

```bash
./scripts/migrate.sh up
```

3. 启动 API Service：

```bash
go run ./cmd/api-service
```

4. 打开：

```text
http://localhost:8080/
```

5. 使用 JWT 创建任务，确认页面出现 `TASK_CREATED`。
6. 通过数据库或 worker 后续写入 `STEP_STARTED`、`STEP_COMPLETED`，确认页面实时追加事件。

## 11. 边界与后续工作

1. 当前 worker 还未正式接入 EventService，后续执行 step 时应统一调用 `EventService.Append`。
2. Redis Pub/Sub 不提供持久化，可靠补偿依赖 PostgreSQL timeline 查询；如果后续需要可回放消息流，可切换到 NATS JetStream。
3. SSE 长连接通过 `http.ResponseController` 清理写 deadline，避免服务级 `WRITE_TIMEOUT` 提前关闭流式响应。
4. 事件 output 大对象外置对象存储尚未实现，后续 artifact 模块完成后可补充。
