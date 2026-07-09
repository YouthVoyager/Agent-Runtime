# StableAgent 管理台补齐技术文档

## 1. 实现背景

本次实现依据以下设计文档完成：

- `docs/admin-console-design.md`：定义管理台页面结构、角色权限矩阵、页面交互和验收标准。
- `docs/admin-console-api-contract.md`：定义前后端唯一 API 契约，包括统一响应、分页、租户隔离、角色权限和新增管理接口。

实现目标是让管理台从“前端页面骨架 + 部分任务 API”补齐为可访问的管理台闭环：后端提供设计契约中的管理类接口，前端提供路由入口和缺失页面，任务扩展能力可被任务详情页调用。

## 2. 后端实现

### 2.1 路由注册

`internal/app/api/routes.go` 新增 `registerAdminConsoleRoutes` 调用，并复用 API Service 初始化阶段创建的 PostgreSQL 连接池和审计日志服务。

现在 API Service 启动时会注册三类路由：

1. 公共路由：健康检查和服务信息。
2. 任务路由：任务创建、列表、详情、事件、SSE、任务子资源和审批动作。
3. 管理台路由：Dashboard、Approvals、Tools、Tool Policies、Tool Calls、Observability、Artifacts、Tenants、Users、Settings、Audit Logs、Task Templates。

### 2.2 任务扩展接口

`internal/app/api/task_handler.go` 和 `internal/usecase/taskapi/control.go` 现已对齐任务扩展契约：

- `POST /api/v1/tasks/{task_id}/pause`
- `POST /api/v1/tasks/{task_id}/resume-from-checkpoint`
- `GET /api/v1/tasks/{task_id}/state`

这些接口复用用例层的权限控制：

- owner / tenant_admin / platform_admin 可控制任务。
- auditor 只读，不能执行 pause/resume/cancel。
- `GET state` 会对 JSON 字段递归脱敏，普通 user 只拿摘要字段，管理员和 auditor 可拿完整脱敏 state。

任务控制和审批动作已补充 `ip`、`user_agent`，用于写入 `audit_logs`。

### 2.3 任务列表扩展过滤

`internal/usecase/taskapi/service.go` 将任务列表查询从固定 sqlc 查询扩展为动态过滤查询，支持契约中的新增参数：

- `goal`
- `user_id`
- `tenant_id`
- `created_from`
- `created_to`
- `waiting_approval`
- `stopped_by_limit`
- `status`

隔离规则仍在服务层统一收敛：

- `platform_admin` 可按 `tenant_id` 跨租户过滤，不传则可查全平台。
- 非 `platform_admin` 强制限定 token 中的 `tenant_id`。
- 普通 `user` 强制限定 `user_id = sub`。

任务摘要响应补充 `tenant_id` 和 `user_id`，方便管理台列表展示和运维排查。

### 2.4 管理台 API Handler

新增 `internal/app/api/admin_handler.go`，集中实现管理台新增接口。该文件的职责是 HTTP 入参解析、权限校验、轻量聚合查询、响应映射和审计日志写入。

主要能力如下：

- Dashboard：
  - `/dashboard/summary`
  - `/dashboard/task-stats`
  - `/dashboard/cost-stats`
  - `/dashboard/risk-stats`
  - `/dashboard/system-health`
- Approvals：
  - `/approvals`
  - `/approvals/pending`
  - `/approvals/{approval_id}`
- Tools：
  - `/tools`
  - `/tools/available`
  - `/tools/{tool_name}`
  - `PATCH /tools/{tool_name}`
- Tool Policies：
  - CRUD
  - `/tool-policies/simulate`
- Tool Calls：
  - 跨任务列表
  - 单条详情
- Observability：
  - `/events`
  - `/observability/trace/{trace_id}`
  - `/observability/metrics`
  - `/observability/logs`
- Artifacts：
  - 跨任务列表
  - 详情
  - 下载
  - 管理员删除
- Tenants：
  - 列表
  - 创建
  - 详情
  - 更新
- Users：
  - 列表
  - 邀请
  - 更新
- Settings：
  - 全部分组读取
  - 单分组覆盖更新
- Audit Logs：
  - 审计日志分页查询
- Task Templates：
  - 列表
  - 创建
  - 删除

### 2.5 权限与数据隔离

后端统一使用 `internal/security/authz/role.go` 中的角色函数：

- `NormalizeRole`
- `IsPlatformAdmin`
- `IsTenantAdmin`
- `IsAuditor`
- `IsAdmin`
- `EffectiveTenantID`
- `ScopeToSelf`

管理台 handler 中按资源类型生成隔离条件：

- 任务类：`agent_tasks.tenant_id` + 普通用户 `agent_tasks.user_id`
- 工具调用类：`tool_calls.tenant_id` + 普通用户 `tool_calls.user_id`
- 审批类：`approvals.tenant_id` + 普通用户关联 `tool_calls.user_id`
- 事件类：`agent_events.tenant_id` + 普通用户关联 `agent_tasks.user_id`
- 产物类：`artifacts.tenant_id` + 普通用户关联 `agent_tasks.user_id`

写操作权限：

- 工具、工具策略、用户、产物删除：tenant_admin / platform_admin。
- 租户创建和更新、系统设置更新：platform_admin。
- auditor 全局只读。

### 2.6 脱敏策略

所有返回原始 JSON 的接口使用 `pkg/jsonx.RedactJSON` 递归脱敏。

默认敏感关键词包括：

- `token`
- `secret`
- `password`
- `api_key`
- `apikey`
- `authorization`

覆盖范围包括：

- AgentState
- Tool arguments/result
- Event input/output/metadata
- Settings
- Audit before/after
- Artifact metadata/content
- Task template payload

### 2.7 审计日志

`internal/usecase/auditlog` 已接入 API Service，并在以下写操作中记录审计：

- `task.create`
- `task.cancel`
- `task.pause`
- `task.resume`
- `task.resume_from_checkpoint`
- `approval.approve`
- `approval.reject`
- `tool.update`
- `tool_policy.create`
- `tool_policy.update`
- `tool_policy.delete`
- `tenant.create`
- `tenant.update`
- `user.invite`
- `user.update`
- `artifact.delete`
- `settings.update`

审计字段包含 actor、tenant、action、resource、before、after、ip、user_agent。

## 3. 前端实现

### 3.1 React 入口与路由

新增：

- `web-ui/src/main.tsx`
- `web-ui/src/app/router.tsx`

`main.tsx` 挂载 React 根节点、`AppProviders` 和 `RouterProvider`。

`router.tsx` 注册全部管理台路由，并用 `RequireAuth` 和 `RequireRole` 实现登录和页面权限守卫。

### 3.2 已接入页面

路由现在覆盖设计文档中的左侧导航：

- Dashboard
- Tasks
  - Task List
  - Task Create
  - Task Detail
- Approvals
- Tools
  - Tool Registry
  - Tool Policies
  - Tool Calls
- Observability
  - Timeline Explorer
  - Trace Viewer
  - Metrics
  - Logs
- Artifacts
- Tenants
- Users & Roles
- Settings
- Audit Logs

### 3.3 新增页面

新增以下页面文件：

- `web-ui/src/pages/observability/LogsPage.tsx`
- `web-ui/src/pages/tenants/TenantsPage.tsx`
- `web-ui/src/pages/users/UsersPage.tsx`
- `web-ui/src/pages/settings/SettingsPage.tsx`
- `web-ui/src/pages/audit/AuditLogsPage.tsx`

这些页面使用 Ant Design 的 Card、Table、Form、Modal、Tabs、Input、Select 等组件，保持管理台的工作型界面风格。

### 3.4 API 调整

`web-ui/src/api/users.ts` 的 `update` 方法增加可选 `tenantId` 参数，用于 platform_admin 编辑跨租户用户时向后端传递 `tenant_id` 查询参数。

## 4. 验证结果

### 4.1 后端验证

已执行：

```bash
/opt/homebrew/Cellar/go/1.26.4/libexec/bin/go test ./...
```

结果：通过。

覆盖范围包括 API handler、鉴权、事件服务、runtime worker、tool gateway、json 脱敏等现有测试。

### 4.2 前端验证限制

尝试执行前端依赖安装和类型检查：

```bash
/Users/hao/.cache/codex-runtimes/codex-primary-runtime/dependencies/bin/pnpm install
/Users/hao/.cache/codex-runtimes/codex-primary-runtime/dependencies/bin/pnpm exec tsc --noEmit
```

均失败于 npm registry DNS 解析：

```text
ERR_PNPM_META_FETCH_FAIL
GET https://registry.npmjs.org/@types%2Freact: fetch failed
```

提升网络权限安装也被运行环境自动拒绝，因此本次无法完成前端 TypeScript 构建验证。代码已按现有类型和页面模式静态核对，待依赖可安装后需要补跑：

```bash
cd web-ui
pnpm install
pnpm run typecheck
pnpm run build
```

## 5. 后续建议

1. 在可联网环境补跑前端依赖安装、类型检查和构建。
2. 为 `admin_handler.go` 中的管理台聚合接口补充单元测试或集成测试。
3. 后续如果接入真实 Redis/Temporal/Tempo，应将 `/dashboard/system-health` 和 `/observability/trace` 从轻量聚合升级为真实依赖探测。
4. 当前日志页使用 `agent_events` 映射轻量日志，后续可继续合并 `audit_logs` 和外部日志系统查询。
