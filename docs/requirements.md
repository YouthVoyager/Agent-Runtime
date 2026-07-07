# Agent Runtime 需求拆解

## 1. 文档目的

本文档用于冻结 Agent Runtime 项目的第 1 天需求边界，明确项目要解决的问题、核心目标、目标用户、核心用户流程和核心领域实体。后续架构设计、数据库设计、API 设计和工程实现必须以本文档为上游依据。

## 2. 项目解决的问题

Agent 任务通常不是一次简单的模型调用，而是一个多步骤、长时间、可能调用外部工具、可能需要人工审批、可能失败后恢复的执行过程。生产环境中如果只把 Agent 当作同步请求处理，会出现以下问题：

| 问题 | 影响 |
| --- | --- |
| 长任务无法可靠恢复 | Worker 重启、网络抖动、模型失败后任务可能丢失或从头重跑 |
| 工具调用缺少统一管控 | 高风险工具可能绕过权限、审批、审计和幂等保护 |
| 人工审批流程缺失 | Agent 遇到发送邮件、修改数据、删除资源等动作时无法安全等待用户决策 |
| 执行过程不可观测 | 用户和运维人员无法看到任务当前执行到哪一步、为什么暂停或失败 |
| cancel/resume 缺少安全边界 | 任务可能在外部副作用执行中被中断，造成状态和真实世界不一致 |
| 产物缺少统一管理 | 报告、文件、大模型输出、工具结果无法与任务生命周期关联 |

本项目要建设一个生产级 Agent Runtime 平台，用统一的任务生命周期、事件流、工具网关、审批机制、checkpoint 和恢复机制，支撑可追踪、可审批、可恢复、可取消的 Agent 长任务执行。

## 3. 核心目标

### 3.1 业务目标

1. 用户可以创建一个 Agent 长任务，并立即得到任务 ID。
2. Agent 可以异步执行任务，持续产生日志化事件和执行状态。
3. Agent 调用工具时必须经过统一 Tool Gateway。
4. 高风险工具调用必须进入人工审批流程。
5. Worker、LLM 或工具失败后，任务可以基于 checkpoint 恢复。
6. 用户可以安全取消任务，系统在安全边界停止执行。
7. 用户可以查看任务 timeline、当前状态、工具调用、审批结果和产物。

### 3.2 技术目标

1. 任务状态必须可持久化，不能依赖单进程内存。
2. 核心事件必须只追加，不允许被覆盖。
3. 外部工具调用必须具备幂等键，避免恢复时重复触发副作用。
4. checkpoint 必须记录可恢复所需的 AgentState 快照和事件偏移。
5. cancel、resume、approval 必须是幂等操作。
6. 所有核心数据必须带 `tenant_id`，为多租户隔离预留边界。
7. 所有关键动作必须能被 trace、日志和 timeline 串联。

## 4. 目标用户

| 用户角色 | 主要诉求 |
| --- | --- |
| 业务用户 | 创建 Agent 任务，查看进度，审批高风险动作，获取最终产物 |
| 平台管理员 | 配置工具权限、审批策略、任务预算和租户策略 |
| 开发人员 | 接入新工具、新模型和新 Agent 执行策略 |
| 运维人员 | 观察任务成功率、失败原因、工具耗时、恢复次数和系统瓶颈 |

## 5. 核心用户流程

### 5.1 创建并执行任务

1. 用户在 UI 或 API 中输入任务目标、预算和约束。
2. API Service 校验身份、租户、参数和预算。
3. 系统创建 `AgentTask`、初始化 `AgentState`、写入 `TASK_CREATED` 事件。
4. 系统异步启动 Runtime Worker。
5. Runtime Worker 执行 Agent 主循环，并持续写入 `AgentEvent`。
6. 任务完成后写入最终状态和 `Artifact`。

### 5.2 查看任务进度

1. 用户打开任务详情页。
2. API Service 返回 `AgentTask` 当前状态、预算用量、当前 step 和最近错误。
3. 用户通过 timeline 查询或 SSE 实时流查看 `AgentEvent`。
4. 用户可以继续查看工具调用、审批记录、checkpoint 和 artifacts。

### 5.3 高风险工具审批

1. Agent 计划调用工具。
2. Runtime Worker 将工具请求发送给 Tool Gateway。
3. Tool Gateway 校验工具 schema、权限和风险等级。
4. 如果风险等级为 HIGH，系统创建 `ToolCall` 和 `Approval`，任务进入 `WAITING_APPROVAL`。
5. 用户在 UI 中选择 approve 或 reject。
6. 系统记录审批结果并唤醒任务。
7. approve 后继续工具执行，reject 后 Agent 根据拒绝结果进入下一步或失败。

### 5.4 失败后恢复

1. 任务因 Worker 重启、LLM 失败、工具失败或预算限制进入可恢复状态。
2. 用户或系统发起 resume。
3. 系统读取最近有效 `Checkpoint`。
4. 系统校验 checkpoint 对应的 `AgentState.version` 和未完成 `ToolCall`。
5. Runtime Worker 从 checkpoint 恢复状态，继续执行后续 step。

### 5.5 安全取消任务

1. 用户发起 cancel。
2. 系统将任务标记为 `CANCEL_REQUESTED`，并写入 cancel flag。
3. Runtime Worker 在安全边界检查 cancel flag。
4. Worker 停止继续执行 LLM 或工具调用。
5. 系统写入 `TASK_CANCELLED` 事件，任务进入 `CANCELLED`。

## 6. 核心实体

### 6.1 AgentTask

`AgentTask` 是任务生命周期的聚合根，代表用户提交的一次 Agent 长任务。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `task_id` | string | 全局唯一任务 ID |
| `tenant_id` | string | 租户 ID，所有查询必须携带 |
| `user_id` | string | 创建任务的用户 ID |
| `goal` | text | 用户提交的任务目标 |
| `status` | enum | 当前任务状态 |
| `budget` | json | 最大 step、token、工具次数、成本等预算 |
| `budget_usage` | json | 已使用 step、token、工具次数、成本 |
| `workflow_id` | string | 工作流引擎 ID，MVP 可为空或等于 task_id |
| `trace_id` | string | 可观测链路 ID |
| `last_error_code` | string | 最近一次错误码 |
| `last_error_message` | text | 最近一次错误信息 |
| `created_at` | timestamp | 创建时间 |
| `updated_at` | timestamp | 更新时间 |

建议状态枚举：

| 状态 | 说明 |
| --- | --- |
| `QUEUED` | 已创建，等待执行 |
| `RUNNING` | 执行中 |
| `WAITING_APPROVAL` | 等待人工审批 |
| `PAUSED` | 已暂停，等待恢复 |
| `FAILED` | 不可自动恢复失败 |
| `RETRYABLE_FAILED` | 可恢复失败 |
| `CANCEL_REQUESTED` | 用户已请求取消，等待安全停止 |
| `CANCELLED` | 已取消 |
| `COMPLETED` | 已完成 |
| `STOPPED_BY_LIMIT` | 因预算或循环限制停止 |

### 6.2 AgentEvent

`AgentEvent` 是任务 timeline 的最小记录单元，只追加，不修改。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `event_id` | int64 | 单调递增事件 ID |
| `task_id` | string | 所属任务 |
| `tenant_id` | string | 所属租户 |
| `type` | enum | 事件类型 |
| `input` | json | 事件输入摘要 |
| `output` | json | 事件输出摘要 |
| `metadata` | json | 附加上下文 |
| `trace_id` | string | trace ID |
| `span_id` | string | span ID |
| `created_at` | timestamp | 创建时间 |

核心事件类型：

| 类型 | 触发时机 |
| --- | --- |
| `TASK_CREATED` | 任务创建成功 |
| `TASK_STARTED` | Worker 开始处理任务 |
| `STEP_STARTED` | Agent step 开始 |
| `LLM_STARTED` | LLM 调用开始 |
| `LLM_COMPLETED` | LLM 调用完成 |
| `TOOL_CALL_REQUESTED` | Agent 请求调用工具 |
| `TOOL_APPROVAL_REQUIRED` | 工具调用需要审批 |
| `TOOL_CALL_COMPLETED` | 工具调用完成 |
| `CHECKPOINT_CREATED` | checkpoint 写入完成 |
| `TASK_WAITING_APPROVAL` | 任务进入等待审批 |
| `TASK_RESUMED` | 任务恢复执行 |
| `TASK_CANCEL_REQUESTED` | 用户请求取消 |
| `TASK_CANCELLED` | 任务取消完成 |
| `TASK_FAILED` | 任务失败 |
| `TASK_COMPLETED` | 任务完成 |
| `ARTIFACT_CREATED` | 产物创建完成 |

### 6.3 AgentState

`AgentState` 保存 Agent 当前执行上下文，用于 step 之间传递状态和 checkpoint 恢复。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `task_id` | string | 任务 ID |
| `tenant_id` | string | 租户 ID |
| `current_plan` | json | 当前计划 |
| `current_step` | int | 当前 step 序号 |
| `memory_summary` | text | 压缩后的上下文记忆 |
| `constraints` | json | 用户约束和系统约束 |
| `artifacts` | json | 已生成产物引用 |
| `loop_fingerprints` | json | 循环检测指纹 |
| `version` | int | 乐观锁版本 |
| `updated_at` | timestamp | 更新时间 |

关键约束：

1. 更新 `AgentState` 必须校验 `version`。
2. checkpoint 必须记录对应的 `state_version`。
3. 恢复时只能从有效版本的 state snapshot 继续。

### 6.4 ToolCall

`ToolCall` 记录一次工具调用请求、风险判断、审批状态和执行结果。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `call_id` | string | 工具调用 ID |
| `task_id` | string | 所属任务 |
| `tenant_id` | string | 所属租户 |
| `user_id` | string | 发起任务的用户 |
| `tool_name` | string | 工具名称 |
| `arguments` | json | 工具入参 |
| `arguments_hash` | string | 规范化参数 hash |
| `idempotency_key` | string | 幂等键 |
| `status` | enum | 工具调用状态 |
| `risk_level` | enum | 风险等级 |
| `approval_status` | enum | 审批状态 |
| `result` | json | 工具结果摘要 |
| `result_artifact_id` | string | 大结果对应 artifact |
| `error_code` | string | 错误码 |
| `error_message` | text | 错误信息 |
| `started_at` | timestamp | 开始时间 |
| `finished_at` | timestamp | 结束时间 |

工具调用状态：

| 状态 | 说明 |
| --- | --- |
| `PENDING` | 已登记，尚未执行 |
| `WAITING_APPROVAL` | 等待审批 |
| `APPROVED` | 已审批通过，等待执行或继续执行 |
| `REJECTED` | 已被拒绝 |
| `RUNNING` | 执行中 |
| `SUCCEEDED` | 执行成功 |
| `FAILED` | 执行失败 |
| `CANCELLED` | 因任务取消停止 |

风险等级：

| 等级 | 说明 | MVP 处理 |
| --- | --- | --- |
| `LOW` | 只读工具 | 自动执行 |
| `MEDIUM` | 可回滚写操作 | 默认自动执行，保留策略接口 |
| `HIGH` | 外部可见或不可逆操作 | 必须人工审批 |
| `CRITICAL` | 资金、生产配置、权限变更、批量删除 | MVP 默认拒绝 |

### 6.5 Checkpoint

`Checkpoint` 是任务恢复的稳定锚点，记录某个安全边界的状态快照。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `checkpoint_id` | string | checkpoint ID |
| `task_id` | string | 所属任务 |
| `tenant_id` | string | 所属租户 |
| `state_version` | int | 对应的 AgentState 版本 |
| `event_offset` | int64 | 创建 checkpoint 时最新 event_id |
| `state_snapshot` | json | 可恢复状态快照 |
| `reason` | enum | checkpoint 原因 |
| `trace_id` | string | trace ID |
| `created_at` | timestamp | 创建时间 |

checkpoint 原因：

| 原因 | 说明 |
| --- | --- |
| `BEFORE_LLM` | LLM 调用前 |
| `AFTER_LLM` | LLM 调用后 |
| `BEFORE_TOOL` | 工具调用前 |
| `AFTER_TOOL` | 工具调用后 |
| `STEP_COMPLETED` | step 完成后 |
| `BEFORE_APPROVAL_WAIT` | 进入审批等待前 |
| `MANUAL_PAUSE` | 人工暂停前 |

### 6.6 Approval

`Approval` 记录高风险工具调用的人工决策。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `approval_id` | string | 审批 ID |
| `task_id` | string | 所属任务 |
| `tenant_id` | string | 所属租户 |
| `call_id` | string | 关联 ToolCall |
| `approver_id` | string | 审批人 |
| `status` | enum | 审批状态 |
| `risk_level` | enum | 风险等级 |
| `approval_reason` | text | 系统生成的审批原因 |
| `comment` | text | 审批人备注 |
| `expires_at` | timestamp | 过期时间 |
| `decided_at` | timestamp | 决策时间 |
| `created_at` | timestamp | 创建时间 |

审批状态：

| 状态 | 说明 |
| --- | --- |
| `PENDING` | 等待审批 |
| `APPROVED` | 已通过 |
| `REJECTED` | 已拒绝 |
| `EXPIRED` | 已过期 |
| `CANCELLED` | 因任务取消关闭 |

关键约束：

1. 一个 `ToolCall` 只能有一个有效 `Approval`。
2. approve/reject 必须幂等，多次提交返回同一最终结果。
3. 审批决策必须写入 `AgentEvent`。

### 6.7 Artifact

`Artifact` 记录任务生成的可下载或可引用产物。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `artifact_id` | string | 产物 ID |
| `task_id` | string | 所属任务 |
| `tenant_id` | string | 所属租户 |
| `type` | enum | 产物类型 |
| `name` | string | 展示名称 |
| `storage_uri` | string | 对象存储 URI 或本地 URI |
| `mime_type` | string | MIME 类型 |
| `size_bytes` | int64 | 文件大小 |
| `checksum` | string | 校验值 |
| `metadata` | json | 产物元数据 |
| `created_at` | timestamp | 创建时间 |

产物类型：

| 类型 | 说明 |
| --- | --- |
| `REPORT` | 报告类结果 |
| `FILE` | 普通文件 |
| `TOOL_RESULT` | 工具大结果 |
| `LOG_EXPORT` | 日志或 timeline 导出 |
| `STATE_SNAPSHOT` | 状态快照归档 |

## 7. 功能需求

### 7.1 任务管理

| 编号 | 需求 | 优先级 |
| --- | --- | --- |
| FR-001 | 创建任务并返回 `task_id` | P0 |
| FR-002 | 查询任务详情 | P0 |
| FR-003 | 查询任务 timeline | P0 |
| FR-004 | 取消任务 | P0 |
| FR-005 | 从 checkpoint 恢复任务 | P0 |
| FR-006 | 查询任务 artifacts | P1 |

### 7.2 Runtime 执行

| 编号 | 需求 | 优先级 |
| --- | --- | --- |
| FR-101 | Worker 异步执行 Agent 主循环 | P0 |
| FR-102 | 每个 step 写入开始和完成事件 | P0 |
| FR-103 | LLM 调用前后写入 checkpoint | P0 |
| FR-104 | 工具调用前后写入 checkpoint | P0 |
| FR-105 | 检查 step、token、tool call 预算 | P0 |
| FR-106 | 检测 cancel flag 并在安全边界停止 | P0 |

### 7.3 工具与审批

| 编号 | 需求 | 优先级 |
| --- | --- | --- |
| FR-201 | 工具调用必须经过 Tool Gateway | P0 |
| FR-202 | Tool Gateway 校验工具名称和参数 schema | P0 |
| FR-203 | Tool Gateway 计算风险等级 | P0 |
| FR-204 | HIGH 风险工具必须创建审批 | P0 |
| FR-205 | CRITICAL 风险工具 MVP 默认拒绝 | P0 |
| FR-206 | 工具调用必须使用幂等键 | P0 |

### 7.4 事件与产物

| 编号 | 需求 | 优先级 |
| --- | --- | --- |
| FR-301 | 所有核心动作写入 `AgentEvent` | P0 |
| FR-302 | timeline 支持按 `task_id + event_id` 分页查询 | P0 |
| FR-303 | 任务完成后可生成 `Artifact` | P1 |
| FR-304 | 大输出通过 Artifact 引用，不直接塞进 event | P1 |

## 8. 非功能需求

| 维度 | 要求 |
| --- | --- |
| 可靠性 | 任务创建、事件写入、checkpoint 写入、审批决策必须持久化 |
| 幂等性 | 创建任务、工具调用、审批、cancel、resume 必须幂等 |
| 可恢复性 | Worker 重启后可以从最近 checkpoint 恢复 |
| 可观测性 | 每个任务具备 `trace_id`，核心事件可查询 |
| 安全性 | 所有核心数据按 `tenant_id` 隔离，工具调用做权限和风险控制 |
| 可扩展性 | MVP 保留服务拆分边界，后续可独立拆分 LLM Gateway、Event Service、Approval Service |
| 性能 | API 创建任务不等待 Agent 完成，必须异步执行 |

## 9. 验收标准映射

| 验收项 | 本文档对应内容 |
| --- | --- |
| 能清楚说明项目解决什么问题 | 第 2 节 |
| 能明确核心目标 | 第 3 节 |
| 能输出核心用户流程 | 第 5 节 |
| 能梳理核心实体 | 第 6 节 |
| 能为 MVP 范围提供依据 | 第 7 节、第 8 节 |

