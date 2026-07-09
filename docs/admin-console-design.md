# StableAgent 管理页面完整设计方案

## 一、管理台总体定位

### 1. 管理台目标

StableAgent 管理台用于管理生产级通用 Agent 的完整生命周期，包括：

* 创建 Agent 任务；
* 查看任务执行状态；
* 实时观察执行 timeline；
* 审批高风险工具调用；
* 控制任务 cancel / pause / resume；
* 查看 checkpoint 和恢复点；
* 查看工具调用记录；
* 管理工具权限和风险策略；
* 查看 token、step、成本和循环检测；
* 查看日志、trace、metrics；
* 管理租户、用户和角色；
* 管理系统配置。

---

## 二、页面结构总览

推荐左侧导航如下：

```text
StableAgent Console
├── Dashboard              总览大盘
├── Tasks                  任务管理
│   ├── Task List          任务列表
│   ├── Task Create        创建任务
│   └── Task Detail        任务详情
├── Approvals              审批中心
├── Tools                  工具管理
│   ├── Tool Registry      工具注册表
│   ├── Tool Policies      工具策略
│   └── Tool Calls         工具调用记录
├── Observability          可观测性
│   ├── Timeline Explorer  执行事件查询
│   ├── Trace Viewer       Trace 查询
│   ├── Metrics            指标监控
│   └── Logs               日志查询
├── Artifacts              任务产物
├── Tenants                租户管理
├── Users & Roles          用户与权限
├── Settings               系统设置
└── Audit Logs             审计日志
```

---

## 三、角色与页面权限

### 1. 角色设计

| 角色             | 说明                      |
| -------------- | ----------------------- |
| USER           | 普通用户，可以创建和管理自己的任务       |
| TENANT_ADMIN   | 租户管理员，可以管理租户内任务、用户、工具策略 |
| APPROVER       | 审批人，可以审批高风险工具调用         |
| AUDITOR        | 审计员，只读查看任务、事件、工具调用、日志   |
| PLATFORM_ADMIN | 平台管理员，可以管理所有租户和系统配置     |
| SYSTEM_WORKER  | 系统 Worker 身份，不进入 UI     |

### 2. 页面权限矩阵

| 页面        | USER | APPROVER | AUDITOR | TENANT_ADMIN | PLATFORM_ADMIN |
| --------- | ---: | -------: | ------: | -----------: | -------------: |
| Dashboard |    是 |        是 |       是 |            是 |              是 |
| 创建任务      |    是 |       可选 |       否 |            是 |              是 |
| 任务列表      |  自己的 |    待审批相关 |      只读 |          租户内 |             全部 |
| 任务详情      |  自己的 |     审批相关 |      只读 |          租户内 |             全部 |
| 审批中心      | 相关审批 |        是 |      只读 |          租户内 |             全部 |
| 工具管理      |    否 |        否 |      只读 |            是 |              是 |
| 可观测页面     | 自己相关 |     相关任务 |      只读 |          租户内 |             全部 |
| 租户管理      |    否 |        否 |       否 |           部分 |              是 |
| 用户权限      |    否 |        否 |       否 |          租户内 |             全部 |
| 系统设置      |    否 |        否 |       否 |           部分 |              是 |
| 审计日志      |    否 |        否 |       是 |            是 |              是 |

---

## 四、核心页面设计

### 1. Dashboard 总览页

#### 1.1 页面目标

让用户和管理员快速了解当前 Agent 平台运行状态。

#### 1.2 核心功能

任务概览：今日创建任务数、运行中任务数、等待审批任务数、失败任务数、已完成任务数、被超限停止任务数、平均任务耗时、任务成功率。

成本概览：今日 token 消耗、今日 LLM 成本、平均单任务成本、工具调用次数、高风险工具调用次数、被审批拦截次数。

风险概览：待审批数量、高风险工具调用趋势、被拒绝工具调用数量、触发 loop detector 的任务数、token 超限任务数、step 超限任务数。

系统健康：API Service 状态、Runtime Worker 状态、Tool Gateway 状态、LLM Gateway 状态、PostgreSQL 状态、Redis 状态、Temporal 状态、Queue backlog、Worker 并发数。

#### 1.3 页面布局

```text
顶部：
- 时间范围选择：最近 1 小时 / 24 小时 / 7 天 / 自定义
- 租户选择器，仅管理员可见
第一行：总任务数、运行中任务、等待审批、失败任务、成功率
第二行：token 消耗趋势图、任务状态分布图、工具调用风险分布图
第三行：最近失败任务、最近待审批、系统健康状态
```

#### 1.4 交互设计

* 点击"运行中任务"跳转任务列表并自动筛选 `RUNNING`；
* 点击"等待审批"跳转审批中心；
* 点击"失败任务"跳转任务列表并筛选 `FAILED`；
* 点击某个系统服务状态跳转可观测页面；
* 时间范围变化后刷新所有图表。

#### 1.5 后端接口

```http
GET /api/v1/dashboard/summary
GET /api/v1/dashboard/task-stats
GET /api/v1/dashboard/cost-stats
GET /api/v1/dashboard/risk-stats
GET /api/v1/dashboard/system-health
```

#### 1.6 验收标准

* 用户进入页面后 2 秒内看到核心统计；
* 数据按 tenant 隔离；
* 管理员可以切换时间范围；
* 点击统计卡片可以跳转到对应明细页面。

---

### 2. Task List 任务列表页

#### 2.1 页面目标

集中查看和筛选所有 Agent 任务。

#### 2.2 核心功能

任务列表字段：Task ID、Goal、Status、User、Tenant、Current Step、Used Tokens、Tool Calls、Cost、Created At、Updated At、Actions（查看、取消、恢复、复制 ID）。

#### 2.3 筛选条件

task_id 精确搜索、goal 关键词搜索、status 多选、user 筛选、tenant 筛选、创建时间范围、是否等待审批、是否失败、是否超限停止、是否触发 loop、risk_level、tool_name。

#### 2.4 操作按钮

| 状态               | 可用操作            |
| ---------------- | --------------- |
| QUEUED           | 查看、取消           |
| RUNNING          | 查看、取消、暂停        |
| WAITING_APPROVAL | 查看、取消、进入审批      |
| PAUSED           | 查看、恢复、取消        |
| FAILED           | 查看、恢复           |
| RETRYABLE_FAILED | 查看、恢复           |
| STOPPED_BY_LIMIT | 查看、调整预算后恢复      |
| CANCELLED        | 查看              |
| COMPLETED        | 查看、查看 artifacts |

#### 2.5 页面交互

支持自动刷新、手动刷新、保存筛选条件、复制 task_id、批量取消任务（仅管理员）、导出任务列表（仅管理员和审计员）。

#### 2.6 后端接口

```http
GET /api/v1/tasks
POST /api/v1/tasks/{task_id}/cancel
POST /api/v1/tasks/{task_id}/resume
POST /api/v1/tasks/{task_id}/pause
```

#### 2.7 验收标准

* 普通用户只能看到自己的任务；
* 管理员可以看到租户内任务；
* 查询必须按 tenant 隔离；
* 任务状态变化后列表能及时刷新；
* 批量操作必须有二次确认。

---

### 3. Task Create 创建任务页

#### 3.1 页面目标

让用户提交一个新的通用 Agent 任务。

#### 3.2 表单字段

基础信息：

| 字段        | 类型   | 必填 | 说明                  |
| --------- | ---- | -: | ------------------- |
| Goal      | 多行文本 |  是 | 用户希望 Agent 完成的目标    |
| Task Name | 文本   |  否 | 方便用户识别              |
| Priority  | 下拉   |  否 | low / normal / high |
| Tags      | 标签   |  否 | 方便分类                |

执行预算：

| 字段             |    默认值 | 说明       |
| -------------- | -----: | -------- |
| Max Steps      |     50 | 最大执行步骤   |
| Max Tokens     | 200000 | 最大 token |
| Max Tool Calls |     40 | 最大工具调用次数 |
| Max Cost USD   |     10 | 最大成本     |
| Timeout        |  30min | 总任务超时时间  |

执行约束：Language（输出语言）、Require Approval（高风险工具是否必须审批）、Allowed Tools、Disallowed Tools、Output Format（markdown / json / file）、Memory Mode（none / summary / full）、Strict Mode。

高级配置：Model、Tool Policy、Retry Policy、Webhook URL、Artifact Storage。

#### 3.3 交互设计

* Goal 为空时不能提交；
* budget 超出租户限制时提示；
* 高风险工具默认需要审批；
* 选择工具时展示风险等级；
* 提交后跳转到任务详情页；
* 支持保存为任务模板。

#### 3.4 后端接口

```http
POST /api/v1/tasks
GET /api/v1/tools/available
GET /api/v1/task-templates
POST /api/v1/task-templates
```

#### 3.5 验收标准

* 创建成功后返回 task_id；
* 创建任务时写入 TASK_CREATED event；
* 不允许普通用户绕过租户默认预算；
* 不允许选择无权限工具。

---

### 4. Task Detail 任务详情页

#### 4.1 页面目标

展示单个任务的完整执行过程和控制入口。

#### 4.2 页面结构

```text
Task Detail
├── 基本信息区
├── 状态控制区
├── Budget 使用情况
├── Timeline
├── Current State
├── Tool Calls
├── Checkpoints
├── Artifacts
├── Trace 信息
└── Error 信息
```

#### 4.3 基本信息区

展示：task_id、task name、goal、status、user、tenant、workflow_id、trace_id、created_at、updated_at、started_at、finished_at。

支持操作：复制 task_id、复制 trace_id、查看原始 JSON、打开 trace viewer。

#### 4.4 状态控制区

| 状态               | 操作                   |
| ---------------- | -------------------- |
| RUNNING          | Cancel、Pause         |
| PAUSED           | Resume、Cancel        |
| FAILED           | Resume               |
| STOPPED_BY_LIMIT | Adjust Budget、Resume |
| WAITING_APPROVAL | View Approval、Cancel |
| COMPLETED        | View Artifacts       |
| CANCELLED        | 无                    |

所有高风险操作都需要二次确认。

#### 4.5 Budget 使用情况

展示 used_steps / max_steps、used_tokens / max_tokens、used_tool_calls / max_tool_calls、used_cost / max_cost、当前是否接近上限、超限原因。建议用进度条展示。

#### 4.6 Timeline

这是任务详情页最重要的部分。

| Event Type             | 展示内容                     |
| ---------------------- | ------------------------ |
| TASK_CREATED           | 创建时间、创建人、goal            |
| TASK_STARTED           | Worker、workflow_id       |
| STEP_STARTED           | step 编号、当前计划             |
| LLM_CALL_STARTED       | model、prompt 摘要          |
| LLM_CALL_COMPLETED     | token、cost、耗时            |
| TOOL_CALL_STARTED      | tool_name、arguments 摘要   |
| TOOL_APPROVAL_REQUIRED | 风险等级、审批原因                |
| TOOL_CALL_COMPLETED    | result 摘要、耗时             |
| CHECKPOINT_CREATED     | checkpoint_id、reason     |
| LOOP_DETECTED          | fingerprint、重复次数         |
| LIMIT_EXCEEDED         | 超限类型                     |
| TASK_FAILED            | error_code、error_message |
| TASK_RESUMED           | checkpoint_id            |
| TASK_CANCELLED         | 操作人                      |
| TASK_COMPLETED         | artifacts                |

Timeline 功能：实时追加、支持暂停自动滚动、按 event type 筛选、展开 input/output、复制 event JSON、跳转相关 ToolCall、跳转相关 Checkpoint、跳转 trace span。

#### 4.7 Current State

展示当前 AgentState：current_plan、current_step、memory_summary、constraints、artifacts、loop_fingerprints、version、updated_at。

注意：普通用户看到摘要；管理员可以查看完整 JSON；敏感字段必须脱敏。

#### 4.8 Tool Calls

字段：call_id、tool_name、risk_level、status、approval_status、arguments 摘要、result 摘要、idempotency_key、duration、created_at。

支持：查看参数、查看结果、查看审批记录、查看错误、复制 call_id、管理员重放低风险工具调用（默认不开放）。

#### 4.9 Checkpoints

字段：checkpoint_id、reason（BEFORE_LLM / AFTER_LLM / BEFORE_TOOL / AFTER_TOOL）、state_version、event_offset、created_at。

支持操作：查看 snapshot、对比两个 checkpoint、从某个 checkpoint 恢复（仅管理员或 owner）、下载 checkpoint JSON。

生产建议：默认只展示最近 20 个；旧 checkpoint 从对象存储加载；恢复操作必须二次确认。

#### 4.10 Artifacts

展示任务产物：markdown 报告、JSON 结果、文件、图片、表格、中间结果、最终输出。

支持：预览、下载、复制链接、查看生成来源 event、查看 artifact metadata。

#### 4.11 后端接口

```http
GET /api/v1/tasks/{task_id}
GET /api/v1/tasks/{task_id}/events
GET /api/v1/tasks/{task_id}/events/stream
GET /api/v1/tasks/{task_id}/state
GET /api/v1/tasks/{task_id}/tool-calls
GET /api/v1/tasks/{task_id}/checkpoints
GET /api/v1/tasks/{task_id}/artifacts
POST /api/v1/tasks/{task_id}/cancel
POST /api/v1/tasks/{task_id}/pause
POST /api/v1/tasks/{task_id}/resume
POST /api/v1/tasks/{task_id}/resume-from-checkpoint
```

---

### 5. Approvals 审批中心

#### 5.1 页面目标

集中处理所有高风险工具调用。

#### 5.2 页面分类

Pending（待审批）、Approved（已通过）、Rejected（已拒绝）、Expired（已过期）、My Decisions（我的审批记录）。

#### 5.3 审批列表字段

approval_id、task_id、tool_name、risk_level、approval_reason、requester、approver、status、created_at、expires_at。

#### 5.4 审批详情

展示：Agent 想做什么、工具名称、工具参数、风险等级、风险原因、任务 goal、当前 step、上下文摘要、可能影响、幂等键、审批历史。

按钮：Approve、Reject、Request Changes、View Task、View Tool Call。

#### 5.5 审批操作

Approve 需要填写：审批备注（可选）、是否只批准本次、是否后续同类工具自动批准（管理员可选）。

Reject 需要填写：拒绝原因（必填）、是否允许 Agent 继续调整计划、是否直接终止任务。

#### 5.6 后端接口

```http
GET /api/v1/approvals/pending
GET /api/v1/approvals/{approval_id}
POST /api/v1/tool-calls/{call_id}/approve
POST /api/v1/tool-calls/{call_id}/reject
```

#### 5.7 验收标准

* 高风险工具必须出现在审批中心；
* 未审批前工具不能执行；
* 重复审批只生效一次；
* 无权限用户不能审批；
* 审批结果必须写入 AgentEvent 和 AuditLog。

---

### 6. Tool Registry 工具注册表

#### 6.1 页面目标

管理系统支持的所有工具。

#### 6.2 工具列表字段

Tool Name、Description、Category、Risk Level、Enabled、Version、Owner、Created At。

#### 6.3 工具详情

展示：工具名称、工具描述、参数 schema、返回 schema、默认风险等级、支持的租户、执行超时、重试策略、是否有副作用、是否需要幂等键、最近调用记录、错误率、平均耗时。

#### 6.4 工具操作

管理员可操作：启用工具、禁用工具、修改默认风险等级、修改参数 schema、修改超时时间、修改重试策略、查看调用记录、测试低风险工具。

不建议在 UI 直接执行高风险工具测试，除非经过审批流程。

---

### 7. Tool Policies 工具策略页

#### 7.1 页面目标

配置不同租户、角色、工具的使用策略。

#### 7.2 策略维度

Tenant、Role、Tool、Risk Level、Approval Required、Max Calls Per Day、Allowed Arguments、Denied Arguments、Timeout。

#### 7.3 策略示例

```json
{
  "tenant_id": "tenant_001",
  "tool_name": "send_email",
  "role": "USER",
  "enabled": true,
  "approval_required": true,
  "max_calls_per_day": 20,
  "risk_level_override": "HIGH",
  "argument_rules": {
    "allowed_domains": ["company.com"],
    "deny_external_recipients": true
  }
}
```

#### 7.4 页面功能

新建策略、编辑策略、禁用策略、复制策略、查看策略命中记录、查看最近被拒绝调用、查看策略模拟结果。

#### 7.5 策略模拟器

输入：tenant、user、role、tool_name、arguments。
输出：是否允许、risk_level、是否需要审批、命中的策略、拒绝原因。

---

### 8. Tool Calls 工具调用记录页

#### 8.1 页面目标

审计和排查所有工具调用。

#### 8.2 筛选条件

task_id、call_id、tool_name、status、risk_level、approval_status、user、tenant、时间范围、error_code、idempotency_key。

#### 8.3 列表字段

call_id、task_id、tool_name、risk_level、status、approval_status、duration、error_code、created_at。

#### 8.4 详情页

展示：工具参数、参数 hash、幂等键、工具结果、错误详情、审批详情、关联 event、关联 checkpoint、关联 trace span。

---

### 9. Timeline Explorer 执行事件查询页

#### 9.1 页面目标

跨任务查询 AgentEvent，用于排查和审计。

#### 9.2 查询条件

task_id、event_id、event type、tenant、user、tool_name、error_code、trace_id、时间范围。

#### 9.3 展示字段

event_id、task_id、type、input 摘要、output 摘要、metadata、trace_id、created_at。

#### 9.4 交互

点击 event 跳转任务详情对应位置、支持展开 JSON、复制 event、导出事件、按 trace_id 过滤。

---

### 10. Trace Viewer 页面

#### 10.1 页面目标

从技术链路角度查看任务执行耗时和调用关系。

#### 10.2 功能

根据 task_id 查 trace_id、根据 trace_id 查 span、展示 step 耗时、LLM call 耗时、tool call 耗时、checkpoint 耗时、错误 span、跳转外部 Tempo / Jaeger。

#### 10.3 生产建议

Trace Viewer 页面先做轻量版：展示 trace_id、提供跳转到外部 trace 系统、展示系统内保存的关键 span metadata。完整版本再做内嵌 trace waterfall。

---

### 11. Metrics 指标页

任务指标：task_created_total、task_completed_total、task_failed_total、task_cancelled_total、task_duration_p50/p95/p99、running_tasks、waiting_approval_tasks。

LLM 指标：llm_calls_total、llm_errors_total、llm_latency_p95、llm_tokens_total、llm_cost_total、model_usage_distribution。

工具指标：tool_calls_total、tool_errors_total、tool_latency_p95、tool_risk_distribution、approval_required_total、approval_rejected_total。

Worker 指标：active_workers、workflow_backlog、activity_retry_count、queue_lag、worker_cpu、worker_memory。

---

### 12. Logs 日志查询页

查询条件：task_id、trace_id、span_id、tenant_id、service、level、event_type、error_code、时间范围。

展示字段：timestamp、level、service、message、task_id、trace_id、tenant_id、error_code。

注意：日志页面必须脱敏：LLM prompt 中的敏感信息、tool arguments 中的 token/secret/password、邮件内容、用户隐私数据。

---

### 13. Artifacts 任务产物页

产物类型：markdown、JSON、CSV、PDF、图片、文本报告、中间结果、最终输出。

列表字段：artifact_id、task_id、name、type、size、created_by_event_id、created_at。

功能：预览、下载、复制链接、删除（管理员）、查看来源任务、查看来源 event、权限校验。

---

### 14. Tenants 租户管理页

功能：创建租户、禁用租户、查看租户用量、配置默认预算、配置默认工具策略、配置审批策略、配置模型额度、配置 retention 策略。

租户配置项：max_tasks_per_day、max_tokens_per_day、max_cost_per_day、default_max_steps、default_tool_policy、approval_policy、data_retention_days。

---

### 15. Users & Roles 用户与权限页

用户列表：user_id、email、name、tenant、role、status、last_login_at、created_at。

功能：邀请用户、禁用用户、修改角色、查看用户任务、查看用户审批记录、重置 API Key（完整版本）、绑定 OIDC 身份（完整版本）。

---

### 16. Settings 系统设置页

Runtime 设置：默认 max_steps、默认 max_tokens、默认 timeout、checkpoint 保留数量、loop detector 阈值、worker 并发限制。

LLM 设置：默认模型、fallback 模型、provider 配置、token 单价、请求超时、重试次数、限流规则。

Tool 设置：默认审批策略、高风险工具列表、禁用工具列表、工具超时、工具重试。

安全设置：JWT 过期时间、OIDC 配置、API Key 策略、审计日志保留、敏感字段脱敏规则。

数据保留：event 保留时间、checkpoint 保留时间、artifact 保留时间、logs 保留时间。

---

### 17. Audit Logs 审计日志页

必须记录：创建任务、取消任务、恢复任务、从 checkpoint 恢复、审批通过、审批拒绝、修改工具策略、修改租户配置、修改用户角色、删除 artifact、修改系统设置。

字段：audit_id、actor_id、tenant_id、action、resource_type、resource_id、before、after、ip、user_agent、created_at。

---

## 五、核心 UI 交互设计

### 1. Timeline 实时刷新

推荐使用 SSE。

```text
用户打开任务详情页
  -> 请求历史 events
  -> 建立 SSE 连接
  -> 服务端推送新增 event
  -> 前端按 event_id 去重
  -> 自动追加到 timeline
```

要求：支持断线重连、支持 Last-Event-ID、支持 event 去重、支持暂停自动滚动、支持只看错误事件、支持只看工具调用事件。

### 2. 高风险审批弹窗

当任务进入 `WAITING_APPROVAL` 时：任务详情页顶部显示提醒、Timeline 中显示审批卡片、审批中心显示待办、审批人可以进入详情审批。

审批弹窗必须展示：Agent 想调用什么工具、为什么要调用、调用参数是什么、风险等级是什么、可能产生什么影响、是否可回滚、是否有幂等保护。

### 3. Resume 二次确认

恢复任务时必须提示：将从 checkpoint 恢复、恢复点时间、恢复点状态版本、可能会继续执行后续工具调用、已完成的幂等工具不会重复执行。

### 4. Cancel 二次确认

取消任务时必须提示：任务正在运行中、取消后系统会在安全边界停止、已经完成的工具调用不会回滚。

---

## 六、前端状态设计

### 1. Task 状态展示颜色

| 状态               | 建议颜色 | 说明    |
| ---------------- | ---- | ----- |
| QUEUED           | 灰色   | 排队中   |
| RUNNING          | 蓝色   | 执行中   |
| WAITING_APPROVAL | 橙色   | 等待审批  |
| PAUSED           | 黄色   | 已暂停   |
| FAILED           | 红色   | 失败    |
| RETRYABLE_FAILED | 红色   | 可恢复失败 |
| CANCEL_REQUESTED | 灰橙   | 取消中   |
| CANCELLED        | 灰色   | 已取消   |
| COMPLETED        | 绿色   | 已完成   |
| STOPPED_BY_LIMIT | 紫色   | 被限制停止 |

### 2. Event 类型展示

| Event 类型               | UI 样式          |
| ---------------------- | -------------- |
| TASK_CREATED           | 普通信息卡片         |
| STEP_STARTED           | Step 卡片        |
| LLM_CALL_STARTED       | LLM 卡片         |
| LLM_CALL_COMPLETED     | LLM 结果卡片       |
| TOOL_CALL_STARTED      | 工具卡片           |
| TOOL_APPROVAL_REQUIRED | 审批卡片，突出显示      |
| TOOL_CALL_COMPLETED    | 工具结果卡片         |
| CHECKPOINT_CREATED     | Checkpoint 小卡片 |
| LOOP_DETECTED          | 警告卡片           |
| LIMIT_EXCEEDED         | 警告卡片           |
| TASK_FAILED            | 错误卡片           |
| TASK_COMPLETED         | 成功卡片           |

---

## 七、前端技术方案

### 1. 推荐技术栈

React（必须）、TypeScript（必须）、Vite（推荐）、TanStack Query（推荐）、Zustand（可选）、React Router（必须）、Ant Design（推荐）、Monaco Editor（可选）、ECharts / Recharts（推荐）、SSE Client（必须）、OpenAPI Generator（推荐）。

### 2. 前端目录结构

```text
web-ui/
├── src/
│   ├── app/
│   │   ├── router.tsx
│   │   └── providers.tsx
│   ├── pages/
│   │   ├── dashboard/
│   │   ├── tasks/
│   │   ├── approvals/
│   │   ├── tools/
│   │   ├── observability/
│   │   ├── artifacts/
│   │   ├── tenants/
│   │   ├── users/
│   │   ├── settings/
│   │   └── audit/
│   ├── components/
│   │   ├── layout/
│   │   ├── timeline/
│   │   ├── json-viewer/
│   │   ├── status-tag/
│   │   ├── approval-card/
│   │   └── charts/
│   ├── api/
│   │   ├── client.ts
│   │   ├── tasks.ts
│   │   ├── approvals.ts
│   │   ├── tools.ts
│   │   └── observability.ts
│   ├── hooks/
│   │   ├── useTaskEvents.ts
│   │   ├── useSSE.ts
│   │   └── usePermission.ts
│   ├── types/
│   ├── utils/
│   └── main.tsx
└── package.json
```

---

## 八、页面优先级

| 优先级 | 页面                     | 原因         |
| --: | ---------------------- | ---------- |
|  P0 | Task Detail + Timeline | 核心价值，体现可观测 |
|  P0 | Approvals              | 体现高风险工具治理  |
|  P0 | Task List              | 基础管理入口     |
|  P0 | Task Create            | 任务入口       |
|  P0 | Tool Calls             | 排查和审计工具调用  |
|  P1 | Checkpoints            | 体现可恢复能力    |
|  P1 | Dashboard              | 管理概览       |
|  P1 | Artifacts              | 查看任务产物     |
|  P1 | Tool Policies          | 体现生产级治理    |
|  P2 | Trace Viewer           | 运维排查       |
|  P2 | Metrics                | 平台监控       |
|  P2 | Logs                   | 深度排查       |
|  P2 | Tenants / Users        | 企业管理       |
|  P2 | Audit Logs             | 合规和审计      |

---

## 九、最终管理台验收标准

用户侧：可以创建任务、查看自己的任务、看到实时 timeline、取消任务、恢复失败任务、查看任务产物、看到任务失败原因、看到预算使用情况。

审批侧：可以看到待审批工具调用、查看工具参数和风险原因、批准、拒绝、审批结果能驱动任务继续或调整计划、审批操作有审计日志。

管理员侧：可以查看租户内所有任务、配置工具策略、查看工具调用记录、查看失败任务、查看系统指标、查看审计日志、管理用户角色。

运维侧：可以根据 task_id 查 timeline、根据 trace_id 查链路、根据 error_code 查失败原因、查看 LLM 调用耗时、查看工具调用耗时、查看 Worker 状态、查看队列 backlog、查看任务失败率。
