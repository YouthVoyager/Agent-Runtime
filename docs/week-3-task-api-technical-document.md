# 第 3 周 Task API 与任务创建闭环技术文档

## 1. 本周交付范围

本周实现用户 API 的任务创建、任务详情、任务列表、基础 JWT 鉴权和多租户隔离。实现严格遵循 `设计方案.md` 的 API Service、创建任务流程、API 设计、多租户安全要求，以及 `docs/week-2-database-design.md` 中“第 3 周实现任务 API 时应按以下顺序使用本周能力”的说明。

本次交付包含：

1. `POST /api/v1/tasks`：创建 Agent 任务。
2. `GET /api/v1/tasks/{task_id}`：查询任务详情。
3. `GET /api/v1/tasks`：查询任务列表，支持状态过滤、`created_at` 降序排序和分页。
4. HS256 JWT middleware：解析并校验 Bearer token。
5. tenant context 和 user context：从 JWT claims 注入请求上下文。
6. 统一成功响应和统一错误响应。
7. OpenAPI 文档、Postman 集合、Bruno 集合。
8. HTTP handler 集成测试和 JWT 测试。

## 2. 设计依据

`设计方案.md` 对创建任务流程的要求是：

```text
POST /api/v1/tasks
  -> AuthMiddleware 解析 user_id、tenant_id
  -> TaskService 校验预算
  -> DB transaction:
       insert agent_task
       insert agent_state
       insert agent_event: TASK_CREATED
  -> Start Temporal Workflow / 投递队列
  -> 返回 task_id
```

`docs/week-2-database-design.md` 进一步要求第 3 周创建任务时同事务写入 `task_outbox` 的 `START_WORKFLOW` 消息。因此本次实现的真实事务写入顺序是：

1. 写入 `agent_tasks`。
2. 写入初始 `agent_states`。
3. 写入 `agent_events`，事件类型为 `TASK_CREATED`。
4. 写入 `task_outbox`，事件类型为 `START_WORKFLOW`。
5. 提交事务后返回 `task_id` 和 `QUEUED` 状态。

这样做可以保证任务主记录、可恢复状态、审计事件和后续 workflow 启动消息要么全部成功，要么全部失败，不会出现“任务已创建但没有初始状态”或“任务已创建但没有启动补偿消息”的半成功状态。

## 3. 代码结构

主要新增和修改文件如下：

| 文件 | 作用 |
| --- | --- |
| `internal/app/api/routes.go` | API Service 路由注册，初始化 PostgreSQL 连接池并绑定任务路由 |
| `internal/app/api/task_handler.go` | HTTP handler，负责 JSON 解码、上下文读取、参数解析和统一响应 |
| `internal/usecase/taskapi/service.go` | 任务 API 用例层，负责创建事务、查询、权限校验和分页 |
| `internal/domain/task/budget.go` | goal、budget、constraints 领域校验 |
| `internal/domain/task/status.go` | 任务状态白名单和 API 展示转换 |
| `internal/security/authn/jwt.go` | HS256 JWT 解析、签名校验和 middleware |
| `internal/security/authn/context.go` | user context |
| `internal/security/tenant/context.go` | tenant context |
| `internal/security/authz/task.go` | 任务读取和租户级列表权限判断 |
| `internal/transport/http/json.go` | 统一成功响应和统一错误响应 |
| `pkg/ids/ids.go` | 生成全局唯一的 `task_id` 和 `trace_id` |
| `db/queries/tasks.sql` | 补齐创建任务 trace_id 写入、任务列表缺失查询 |
| `api/openapi.yaml` | 第 3 周 Task API OpenAPI 文档 |
| `api/postman/stableagent.postman_collection.json` | Postman 调试集合 |
| `api/bruno/stableagent` | Bruno 调试集合 |

## 4. 鉴权与上下文

### 4.1 JWT 解析

任务 API 使用 `Authorization: Bearer <jwt>`。JWT middleware 位于 `internal/security/authn/jwt.go`，当前支持 HS256：

1. 拆分 JWT 的 header、payload、signature 三段。
2. 校验 header 中 `alg` 必须为 `HS256`。
3. 使用配置中的 `JWT_SECRET` 重新计算 HMAC-SHA256 签名。
4. 使用常量时间比较校验签名，避免签名比较侧信道。
5. 解析 payload 中的 claims。
6. 校验 `tenant_id` 必须存在。
7. 校验 `user_id` 或 `sub` 必须存在。
8. 校验 `exp` 未过期。
9. 校验 `nbf` 未晚于当前时间。

配置项为：

```text
JWT_SECRET
API_SERVICE_JWT_SECRET
```

服务级配置 `API_SERVICE_JWT_SECRET` 优先于通用 `JWT_SECRET`。本地默认值是 `local-dev-secret`，用于本地调试；生产环境必须显式覆盖。

### 4.2 tenant context

middleware 解析 JWT 后，会把 `tenant_id` 写入 `internal/security/tenant` 的 context：

```go
ctx = tenant.WithTenantID(ctx, user.TenantID)
```

后续 handler 不读取请求体里的租户字段，只从 context 获取租户。这样可以避免调用方伪造请求体中的 `tenant_id` 跨租户访问数据。

### 4.3 user context

middleware 同时把用户写入 `internal/security/authn` 的 context：

```go
authn.User{
    TenantID: "tenant_001",
    UserID:   "user_001",
    Role:     "member",
}
```

如果 JWT 没有 `role`，默认角色是 `member`。

## 5. 统一响应格式

成功响应统一为：

```json
{
  "data": {}
}
```

错误响应统一为：

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "goal 不能为空",
    "request_id": "req-123"
  }
}
```

`request_id` 来自 `X-Request-ID` 请求头；如果调用方没有传入，服务会自动生成并写入响应头。

## 6. 创建任务实现

入口是 `POST /api/v1/tasks`。

请求体：

```json
{
  "goal": "分析需求文档并生成生产级 Go 技术方案",
  "budget": {
    "max_steps": 50,
    "max_tokens": 200000,
    "max_tool_calls": 40,
    "max_cost_usd": 10
  },
  "constraints": {
    "language": "zh-CN",
    "require_approval_for_high_risk_tools": true
  }
}
```

### 6.1 参数校验

handler 做 HTTP 层校验：

1. 请求体最大 1 MiB。
2. JSON 必须是单个对象。
3. 未知字段会被拒绝。

用例层做业务校验：

1. `goal` 去掉前后空白后不能为空。
2. `goal` 最长 8000 个字符。
3. `budget.max_steps` 必须在 1 到 1000 之间。
4. `budget.max_tokens` 必须在 1 到 10000000 之间。
5. `budget.max_tool_calls` 必须在 1 到 10000 之间。
6. `budget.max_cost_usd` 必须大于 0 且不超过 100000。
7. `constraints` 可省略；如果提供，必须是 JSON object。

### 6.2 ID 生成

`pkg/ids.New(prefix)` 生成 ID，格式为：

```text
<prefix>_<base36_unix_millis>_<base32_random>
```

创建任务会生成：

1. `task_id`：写入 `agent_tasks.task_id`。
2. `trace_id`：写入 `agent_tasks.trace_id`，并写入 `TASK_CREATED` event 和 outbox payload。

### 6.3 数据库事务

创建任务由 `internal/usecase/taskapi.Service.CreateTask` 完成，使用 `pgx` transaction。事务内执行：

1. `CreateAgentTask`
   - `status = QUEUED`
   - `budget` 保存请求预算
   - `budget_usage` 初始化为 0
   - `trace_id` 保存本次任务 trace
2. `CreateAgentState`
   - `current_plan = {}`
   - `current_step = 0`
   - `constraints = 请求 constraints 或 {}`
   - `artifacts = []`
   - `loop_fingerprints = []`
3. `CreateAgentEvent`
   - `type = TASK_CREATED`
   - `input` 包含 goal、budget、constraints
   - `metadata` 包含 user_id、request_id
4. `CreateTaskOutboxMessage`
   - `aggregate_type = agent_task`
   - `aggregate_id = task_id`
   - `event_type = START_WORKFLOW`
   - `status = PENDING`

如果任一步失败，事务回滚。

## 7. 查询任务详情

入口是 `GET /api/v1/tasks/{task_id}`。

查询逻辑：

1. 从 JWT context 获取 `tenant_id`、`user_id`、`role`。
2. 使用 `tenant_id + task_id` 查询 `agent_tasks`。
3. 如果查询不到，返回 `NOT_FOUND`。
4. 如果任务存在但不是当前用户创建，且当前用户不是 `admin` 或 `owner`，返回 `FORBIDDEN`。
5. 使用 `tenant_id + task_id` 查询 `agent_states` 获取 `current_step`。
6. 返回任务详情。

返回字段包含：

1. `task_id`
2. `goal`
3. `status`
4. `current_step`
5. `budget`
6. `budget_usage`
7. `trace_id`
8. `last_error_code`
9. `last_error_message`
10. `created_at`
11. `updated_at`

## 8. 查询任务列表

入口是 `GET /api/v1/tasks`。

支持参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `status` | 空 | 可选任务状态过滤 |
| `page` | `1` | 页码，从 1 开始 |
| `page_size` | `20` | 每页数量，最大 100 |
| `sort` | `created_at_desc` | 当前支持按创建时间倒序 |

权限逻辑：

1. `admin` 和 `owner` 查询租户内任务。
2. `member` 查询自己创建的任务。
3. 所有 SQL 都带 `tenant_id`。

分页逻辑：

1. 使用 offset pagination：`offset = (page - 1) * page_size`。
2. 实际查询 `page_size + 1` 条。
3. 如果多出 1 条，说明还有下一页，返回 `has_more = true` 和 `next_page`。
4. 对外返回时裁掉多查的第 1 条。

排序逻辑固定为：

```sql
order by created_at desc, task_id desc
```

`task_id desc` 是稳定排序字段，避免相同 `created_at` 下分页结果顺序不稳定。

## 9. 多租户隔离

多租户隔离有三层：

1. JWT middleware 从 token 提取 `tenant_id`，不信任请求体或 query 中的租户字段。
2. 查询任务、状态、列表时 SQL 均带 `tenant_id` 条件。
3. 数据库层已有 `(tenant_id, task_id)` 组合外键和唯一约束，防止跨租户写入关联数据。

创建任务时，如果 JWT 中的 `tenant_id` 和 `user_id` 在数据库中不存在对应用户，`agent_tasks` 的外键会拒绝写入，API 返回 `FORBIDDEN`。

## 10. 闭环演示

本地启动依赖和迁移后，先设置 API 地址和 JWT：

```bash
export BASE_URL=http://localhost:8080
export JWT_TOKEN=<使用 JWT_SECRET 签名的 HS256 token>
```

创建任务：

```bash
curl -sS -X POST "$BASE_URL/api/v1/tasks" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: demo-create-task" \
  -d '{
    "goal": "分析需求文档并生成生产级 Go 技术方案",
    "budget": {
      "max_steps": 50,
      "max_tokens": 200000,
      "max_tool_calls": 40,
      "max_cost_usd": 10
    },
    "constraints": {
      "language": "zh-CN",
      "require_approval_for_high_risk_tools": true
    }
  }'
```

响应中的 `data.task_id` 即为新任务 ID。

查询详情：

```bash
curl -sS "$BASE_URL/api/v1/tasks/$TASK_ID" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "X-Request-ID: demo-get-task"
```

查询列表：

```bash
curl -sS "$BASE_URL/api/v1/tasks?status=QUEUED&page=1&page_size=20&sort=created_at_desc" \
  -H "Authorization: Bearer $JWT_TOKEN" \
  -H "X-Request-ID: demo-list-tasks"
```

这三步覆盖从任务创建、拿到 `task_id`、查询详情、查询列表的闭环。

## 11. 调试方式

OpenAPI 文档：

```text
api/openapi.yaml
```

Postman 集合：

```text
api/postman/stableagent.postman_collection.json
```

Bruno 集合：

```text
api/bruno/stableagent
```

调试前需要设置：

1. `base_url`：默认 `http://localhost:8080`。
2. `jwt_token`：使用 `JWT_SECRET` 或 `API_SERVICE_JWT_SECRET` 签名的 HS256 JWT。
3. `task_id`：创建任务响应中的 ID，用于详情查询。

JWT payload 示例：

```json
{
  "tenant_id": "tenant_001",
  "user_id": "user_001",
  "role": "admin",
  "exp": 1783425600
}
```

## 12. 测试覆盖

本次新增测试：

1. `internal/security/authn/jwt_test.go`
   - JWT 签名校验。
   - 过期 token 拒绝。
   - middleware 写入 user context 和 tenant context。
2. `internal/app/api/task_handler_test.go`
   - `POST /api/v1/tasks` 从 JWT 上下文和请求体构造创建用例输入。
   - 任务 API 缺少 JWT 时返回统一 `UNAUTHORIZED` 错误。
   - `GET /api/v1/tasks` 正确传递 status、page、page_size、sort。

验证命令：

```bash
go test ./...
```

## 13. 当前边界

本周交付完成任务创建到查询的 API 闭环，但 Worker 消费 `task_outbox` 并启动 Temporal Workflow 不在本周实现范围内。当前创建接口已经写入 `START_WORKFLOW` outbox，为后续 Worker/outbox dispatcher 提供可靠补偿入口。
