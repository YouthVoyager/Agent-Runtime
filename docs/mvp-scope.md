# Agent Runtime MVP 范围

## 1. 文档目的

本文档用于冻结 MVP 范围，明确第一个可交付闭环要做什么、不做什么、延后什么，避免在开发过程中随意扩大范围。MVP 的目标不是一次性实现完整生产平台，而是验证 Agent 长任务执行的最小生产闭环。

## 2. MVP 一句话定义

MVP 要实现一个可通过 API 创建 Agent 任务、由 Worker 异步执行、能记录 timeline、能通过 Tool Gateway 调用工具、能对高风险工具进行人工审批、能写入 checkpoint、能 resume、能 cancel、能生成基础 artifact 的最小闭环。

## 3. MVP 成功标准

MVP 交付后，必须可以完整跑通以下链路：

```text
创建任务
  -> Worker 获取任务
  -> Agent 执行 step
  -> 写入 event
  -> 写入 checkpoint
  -> 调用低风险工具并返回结果
  -> 调用高风险工具并等待审批
  -> 用户审批通过
  -> Worker 继续执行
  -> 任务完成并生成 artifact
  -> 用户查看 timeline 和结果
```

同时必须可以跑通异常链路：

```text
任务执行中
  -> 用户 cancel
  -> Worker 在安全边界停止
  -> 任务进入 CANCELLED
```

```text
任务执行失败
  -> 查询最近 checkpoint
  -> resume
  -> Worker 从 checkpoint 继续执行
```

## 4. MVP 范围内功能

### 4.1 API Service

| 功能 | 范围 |
| --- | --- |
| 创建任务 | `POST /api/v1/tasks`，支持 goal、budget、constraints |
| 查询任务 | `GET /api/v1/tasks/{task_id}` |
| 查询 timeline | `GET /api/v1/tasks/{task_id}/events` |
| 审批工具调用 | `POST /api/v1/tool-calls/{call_id}/approve` 和 reject 接口 |
| 取消任务 | `POST /api/v1/tasks/{task_id}/cancel` |
| 恢复任务 | `POST /api/v1/tasks/{task_id}/resume` |
| 查询产物 | `GET /api/v1/tasks/{task_id}/artifacts` |

MVP API 要求：

1. 所有写操作必须校验参数。
2. 所有任务查询必须带 `tenant_id` 过滤。
3. cancel、resume、approve、reject 必须幂等。
4. 创建任务接口不得阻塞等待任务完成。

### 4.2 Runtime Worker

| 功能 | 范围 |
| --- | --- |
| 加载任务 | 根据 `task_id` 加载 `AgentTask` 和 `AgentState` |
| 主循环 | 支持 plan、step、LLM 调用、工具调用、完成判断 |
| 事件写入 | 写入 step、LLM、tool、checkpoint、完成、失败等事件 |
| checkpoint | 在 LLM 和工具调用前后写入 checkpoint |
| 预算检查 | 支持 max_steps、max_tokens、max_tool_calls |
| cancel 检查 | 在安全边界检查 cancel flag |
| resume | 从最近有效 checkpoint 恢复执行 |

MVP 中 Runtime Worker 可以先是单一 worker 进程，但代码边界必须保留未来接入 Temporal 的可能。

### 4.3 Tool Gateway

| 功能 | 范围 |
| --- | --- |
| 工具注册 | 支持静态注册 MVP 工具 |
| 参数校验 | 使用工具 schema 校验参数 |
| 风险识别 | 返回 LOW、MEDIUM、HIGH、CRITICAL |
| 幂等记录 | 使用 `tenant_id + idempotency_key` 防止重复副作用 |
| 审批判断 | HIGH 风险创建审批，CRITICAL 默认拒绝 |
| 结果记录 | 保存工具结果摘要和 artifact 引用 |

MVP 工具建议：

| 工具 | 风险 | 说明 |
| --- | --- | --- |
| `read_document` | LOW | 模拟读取文档 |
| `write_artifact` | MEDIUM | 生成本地或对象存储产物 |
| `send_external_message` | HIGH | 模拟外部可见动作，必须审批 |
| `dangerous_admin_action` | CRITICAL | 默认拒绝，用于验证策略 |

### 4.4 Approval

| 功能 | 范围 |
| --- | --- |
| 创建审批 | HIGH 风险工具调用创建 `Approval` |
| 查询待审批 | 可通过任务详情或 timeline 看到 |
| 通过审批 | approve 后唤醒任务 |
| 拒绝审批 | reject 后将结果返回给 Agent |
| 审批事件 | 写入审批创建和审批决策事件 |

MVP 只做单人审批，不做多级审批。

### 4.5 Checkpoint 与 Resume

| 功能 | 范围 |
| --- | --- |
| 创建 checkpoint | 记录 `state_snapshot`、`state_version`、`event_offset` |
| 查询最近 checkpoint | resume 默认使用最近有效 checkpoint |
| 恢复状态 | 将 checkpoint 中的状态恢复为当前 AgentState |
| 工具幂等校验 | resume 前检查 checkpoint 后是否存在未完成 ToolCall |

MVP 必须保证有副作用工具不会因为 resume 被重复执行。

### 4.6 Event Timeline

| 功能 | 范围 |
| --- | --- |
| 事件追加 | 所有核心动作只追加写入 |
| 分页查询 | 按 `task_id + event_id` 查询 |
| 事件类型 | 覆盖任务、step、LLM、工具、审批、checkpoint、artifact、cancel、resume |
| trace 字段 | 事件保留 `trace_id` 和 `span_id` 字段 |

MVP 可以先不做 SSE，但事件结构要兼容后续实时推送。

### 4.7 Artifact

| 功能 | 范围 |
| --- | --- |
| 创建产物 | 任务完成或工具大结果生成 artifact |
| 查询产物 | 按任务查询产物列表 |
| 存储 URI | MVP 可先使用本地路径或 MinIO/S3 抽象接口 |
| 元数据 | 保存 type、name、mime_type、size、checksum |

## 5. MVP 技术边界

### 5.1 服务边界

设计方案中的生产级服务拆分包括：

```text
api-service
runtime-worker
tool-gateway
llm-gateway
event-service
approval-service
artifact-service
observability-service
web-ui
```

MVP 暂定最小服务形态：

```text
api-service
runtime-worker
tool-gateway
```

其中：

| 能力 | MVP 归属 |
| --- | --- |
| LLM Gateway | 先作为 runtime-worker 内部模块 |
| Event Service | 先作为 api-service/runtime-worker 共用 repository |
| Approval Service | 先作为 api-service 内部 usecase |
| Artifact Service | 先作为 api-service 内部 usecase |
| Observability Service | 先做日志、trace 字段和基础 metrics，不独立成服务 |
| Web UI | MVP 非必需，可通过 API 验证闭环 |

### 5.2 数据存储

MVP 必须设计并使用以下核心表：

```text
agent_tasks
agent_events
agent_states
tool_calls
checkpoints
approvals
artifacts
task_outbox
```

MVP 可以延后以下能力：

```text
事件表分区
checkpoint 冷归档
多区域存储
复杂审计报表
成本账单系统
```

### 5.3 工作流引擎

生产级推荐 Temporal。MVP 的范围约束如下：

| 决策点 | MVP 处理 |
| --- | --- |
| 是否必须引入 Temporal | 第 2 天技术选型最终确认 |
| 如果暂不引入 | 必须保留 `workflow_id`、outbox、resume、幂等边界 |
| 如果引入 | 只实现一个 `AgentTaskWorkflow` 和必要 Activity |

## 6. MVP 明确不做的功能

以下功能不进入 MVP，除非后续单独变更范围：

| 不做项 | 原因 |
| --- | --- |
| 多级审批 | 先验证单人审批闭环，避免审批模型过早复杂化 |
| 企业级 RBAC 管理台 | MVP 只保留租户和用户字段，权限策略后续扩展 |
| 完整 Web UI | API 足以验证闭环，Web UI 放到后续阶段 |
| 多模型复杂路由 | MVP 先支持单模型或一个 LLM Gateway 接口 |
| Prompt 版本管理后台 | 后续与 LLM Gateway 一起完善 |
| 复杂成本账单 | MVP 只记录预算和用量，不生成账单 |
| Kafka/NATS 实时流 | MVP timeline 先通过数据库查询，实时推送后续实现 |
| 事件表分区和归档 | 数据量上来后再做 |
| Kubernetes 部署 | MVP 先满足本地和 Docker Compose |
| 多区域容灾 | 不属于最小闭环 |
| 插件市场 | 工具先静态注册 |
| CRITICAL 工具审批执行 | MVP 默认拒绝，避免高风险误用 |
| 自动规划多 Agent 协作 | MVP 只处理单 Agent 任务 |
| 复杂前端可视化编排 | 先通过 API 和 timeline 验证执行链路 |

## 7. 需求优先级

### 7.1 P0 必须完成

| 编号 | 功能 |
| --- | --- |
| P0-001 | 创建任务 |
| P0-002 | 查询任务详情 |
| P0-003 | 查询 timeline |
| P0-004 | Runtime Worker 执行 Agent 主循环 |
| P0-005 | AgentState 持久化和版本控制 |
| P0-006 | AgentEvent 只追加写入 |
| P0-007 | Tool Gateway 统一工具调用入口 |
| P0-008 | ToolCall 幂等记录 |
| P0-009 | HIGH 风险工具人工审批 |
| P0-010 | checkpoint 创建和查询 |
| P0-011 | resume 从 checkpoint 恢复 |
| P0-012 | cancel 在安全边界停止 |
| P0-013 | 基础 artifact 创建和查询 |

### 7.2 P1 应该完成

| 编号 | 功能 |
| --- | --- |
| P1-001 | SSE 事件流 |
| P1-002 | MinIO/S3 对象存储接入 |
| P1-003 | 基础 metrics |
| P1-004 | outbox worker 补偿机制 |
| P1-005 | 审批过期处理 |
| P1-006 | 工具执行超时和重试 |

### 7.3 P2 后续完成

| 编号 | 功能 |
| --- | --- |
| P2-001 | Web UI 管理台 |
| P2-002 | 多模型路由 |
| P2-003 | 多级审批 |
| P2-004 | 租户级工具策略配置页面 |
| P2-005 | 事件归档和分区 |
| P2-006 | Kubernetes 部署 |

## 8. 范围变更规则

为避免 MVP 范围膨胀，新增功能必须满足以下条件之一才能进入 MVP：

1. 不做该功能，最小闭环无法跑通。
2. 不做该功能，会破坏任务恢复、审批或工具幂等的正确性。
3. 不做该功能，会导致后续核心架构推倒重来。

以下理由不能作为进入 MVP 的充分条件：

1. 后续可能会用到。
2. 生产环境最终会需要。
3. 实现起来看起来不复杂。
4. UI 展示更完整。
5. 可以顺手做掉。

## 9. MVP 最小闭环验收清单

| 验收项 | 通过标准 |
| --- | --- |
| 创建任务 | API 返回 `task_id` 和 `QUEUED` 状态 |
| 异步执行 | API 不等待任务完成，Worker 后台推进任务 |
| timeline | 可以查询到任务创建、step、tool、checkpoint、审批和完成事件 |
| 工具调用 | LOW 工具自动执行，HIGH 工具进入审批 |
| 审批 | approve 后任务继续，reject 后任务按拒绝结果处理 |
| checkpoint | LLM 和工具前后可查到 checkpoint |
| resume | 失败任务可以从最近 checkpoint 继续 |
| cancel | 多次 cancel 幂等，任务最终进入 `CANCELLED` |
| artifact | 完成任务可以查询到产物记录 |
| 边界冻结 | 不做项未进入 MVP 实现 |

## 10. MVP 后的扩展方向

MVP 完成后，优先扩展顺序如下：

1. 独立 LLM Gateway，支持多模型、token 统计、模型降级。
2. 独立 Event Service，支持 SSE、NATS/Kafka、timeline 推送。
3. 接入 Temporal，增强长任务恢复、重试和信号处理。
4. 完善 Tool Gateway，支持动态工具注册、租户策略和工具审计。
5. 建设 Web UI，展示任务详情、timeline、审批卡片和 artifacts。
6. 增强可观测性，接入 OpenTelemetry、Prometheus、Grafana、Jaeger/Tempo。

