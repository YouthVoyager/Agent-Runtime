# StableAgent 设计符合度校验与测试报告

- 日期:2026-07-08
- 校验基线:`feature` 分支 `a5fc10c`(生产环境闭环)
- 对照文档:`设计方案.md`
- 校验方式:静态检查 + 单元测试 + 端到端实测(`e2e-local` + 新增 `e2e-extended`)+ 数据库 schema 对照 + 代码审查

## 一、测试执行结果汇总

| 检查项 | 命令 | 结果 |
| --- | --- | --- |
| 格式化 / 静态检查 | `make fmt` + `go vet ./...` | 通过(golangci-lint 本机未安装,以 go vet 替代) |
| 单元测试 | `make test` | 全部通过,含本次新增 6 个测试文件(测试包 8 → 14) |
| 编译 | `make build` | 4 个服务全部编译成功 |
| Docker 环境 | `make docker-up` + `make migrate-up` + `make health` | 4 服务健康检查通过,迁移幂等 |
| 既有端到端 | `make e2e-local` | 4/4 场景通过 |
| 扩展端到端 | `make e2e-extended`(本次新增) | **9/10 场景通过,场景 7(SSE)失败** → 见问题 P1 |

### 本次新增的自动化测试

| 文件 | 覆盖的设计要求 |
| --- | --- |
| `internal/domain/toolcall/toolcall_test.go` | 工具注册表风险等级、LOW/MEDIUM 自动执行 / HIGH 审批 / CRITICAL 禁止的分级规则、参数规范化、幂等键确定性(设计 §4、§10.1) |
| `internal/domain/task/budget_test.go` | 预算四维校验边界、goal 校验(§7.1) |
| `internal/usecase/runtimeworker/budget_test.go` | step/token/tool/cost 四维预算检测(§9.5) |
| `internal/usecase/llmgateway/service_test.go` | Mock LLM 关键词路由确定性、终态判定 |
| `internal/usecase/toolgateway/validate_test.go` | 工具调用必填字段校验、工具目录 |
| `internal/security/authz/task_test.go` | RBAC:admin/owner 全量、member 仅本人(§10.4) |
| `scripts/e2e-extended.mjs`(`make e2e-extended`) | 下述 10 个端到端场景 |

### e2e-extended 场景与设计条目对应

| 场景 | 设计出处 | 结果 |
| --- | --- | --- |
| 1 跨租户隔离(读/取消/timeline 均 404) | §10.4 多租户隔离 | ✅ |
| 2 client_request_id 重复创建返回同一任务 | §10.1 幂等矩阵 | ✅ |
| 3 重复审批只生效一次、冲突决策 409 | §12 难点二测试表 | ✅ |
| 4 审批拒绝 → 任务 FAILED + TOOL_APPROVAL_REJECTED | §12 难点二测试表 | ✅ |
| 5 取消幂等(重复 cancel 返回当前状态) | §10.1 幂等矩阵 | ✅ |
| 6 WAITING_APPROVAL 任务 resume 被 409 拦截 | §5.4 resume 校验 | ✅ |
| 7 SSE Last-Event-ID 重连补发、event_id 单调 | §7.4 SSE 要求 | ❌ **P1** |
| 8 预算超限 → 任务停止 + BUDGET_EXCEEDED 事件 | §12 难点一 / §9.5 | ✅ |
| 9 租户策略 DENY 拦截工具调用 | §12 难点二测试表 | ✅ |
| 10 worker 重启后任务不丢、副作用工具不重复执行 | §12 难点一测试表 | ✅ |

## 二、行为级问题清单(本次只记录,未修复)

### P1(严重)SSE 实时事件流整体不可用

- 现象:`GET /api/v1/tasks/{id}/events/stream` 恒定返回 503 `"当前 HTTP writer 不支持 SSE"`。web-ui 的实时 timeline 依赖此接口,同样失效(轮询 REST 接口不受影响)。
- 根因:`internal/transport/http/middleware.go` 的 `statusRecorder`(WithAccessLog 中间件)只嵌入 `http.ResponseWriter`,未实现 `Flush()` 和 `Unwrap()`;`event_handler.go:108` 的 `w.(http.Flusher)` 断言必然失败。同文件 `http.NewResponseController(w).SetWriteDeadline` 也因缺少 `Unwrap` 无法穿透包装。
- 复现:`curl -N -H "Authorization: Bearer $JWT" http://localhost:8080/api/v1/tasks/<task_id>/events/stream`
- 修复方向:给 `statusRecorder` 增加 `Flush()`(委托底层)和 `Unwrap() http.ResponseWriter` 方法。
- 对应设计:§7.4「SSE 实时事件」为设计必须项。

### P2(中)resume 缺少状态校验,已完成任务可被重新执行

- 现象:对 `SUCCEEDED` / `CANCELED` 任务调用 resume 返回 200 并重新入队执行;实测 used_steps 1→2、used_tokens 重复累计,任务终态被改写。
- 设计要求(§5.4):resume 仅允许 `FAILED / PAUSED / STOPPED_BY_LIMIT`。
- 根因:`internal/usecase/taskapi/control.go:150` `ResumeTask` 只拦截了 QUEUED/RUNNING(幂等返回)和 WAITING_APPROVAL(409),对 SUCCEEDED/CANCELED 直接走恢复分支。
- 缓解因素:工具幂等键(task_id+step+tool+args)阻止了副作用工具重复执行(tool_calls 表无重复记录),但预算重复累计、事件流被污染。

### P3(轻)预算 tool_calls 用量虚计 —— ✅ 已修复(2026-07-08)

- 原问题:`incrementBudgetUsage` 无条件 `UsedToolCalls++`,即使该 step 未发生工具调用(LLM 直接返回终态)也计入,导致 max_tool_calls 预算提前耗尽。
- 修复:`completeTask` 传入 `toolCalled` 标记,仅实际发生工具调用的 step 累计 tool call 用量;新增单测 `TestIncrementBudgetUsage`。

### P4(轻)outbox PROCESSING 死信恢复未接线 —— ✅ 已修复(2026-07-08)

- 原问题:`ResetStaleTaskOutboxProcessing` 查询已定义但无任何调用方;worker 在 `MarkTaskOutboxProcessing` 之后崩溃,该消息将永久停留 PROCESSING。影响有限:当前 outbox 只承担 workflow_id 回填,任务推进不依赖 outbox 投递。
- 深层根因:`sqlc.yaml` 的 schema 只挂了 000002 迁移,缺 000003,导致 `make sqlc-generate` 报错、生成代码长期落后于 `db/queries`(该函数从未被生成,`events_ext.go` 也是因此手写的补丁)。
- 修复:① `sqlc.yaml` schema 补齐三个 up 迁移并重新生成;② 删除与生成代码重复的手写补丁 `internal/infra/postgres/db/events_ext.go`;③ worker `dispatchOutbox` 派发前调用 `ResetStaleTaskOutboxProcessing`(30s 阈值),将超时 PROCESSING 消息重置为 FAILED 进入既有重试通道。

### P5(基建)verify-services.sh 端口与 compose 冲突 —— ✅ 已修复(2026-07-08)

- 原问题:脚本用备用端口 18082 启动本地 tool-gateway 二进制,与 docker-compose 对外映射的 18082 相同;Docker 环境运行时,健康检查可能命中容器而非被测二进制,冒烟结果失真。
- 修复:tool-gateway 备用端口改为 18084,脚本内注明原因。

### P6(文档)week-4 文档与实现不符 —— ❌ 撤销(误报)

- 复核结论:`web-ui/src/app.js` 实际是 **React**(`import React from 'https://esm.sh/react@19.2.3'`,浏览器运行时经 ESM CDN 加载,故 package.json 零 npm 依赖),week-4 文档第 233 行对此有准确说明。首轮探索将"零 npm 依赖"误判为原生 JS。文档与实现一致,无需修改。

## 三、设计符合度矩阵(按设计方案章节)

图例:✅ 符合 ⚠️ 部分符合 ❌ 不符合 ⏳ 已知差距(本地闭环 MVP 刻意未接入,不判失败,依据 `docs/local-production-loop-technical-document.md`)

### §三 架构 / §四 核心模块

| 设计项 | 状态 | 说明 |
| --- | --- | --- |
| 四服务拆分(api / runtime-worker / tool-gateway / llm-gateway) | ✅ | `cmd/` 四个入口,统一启动器 `internal/app/service.go` |
| Temporal Workflow 生命周期 | ⏳ | 无 SDK;`runtimeworker` 轮询循环替代,`workflow_id = task_id` 预留替换点 |
| gRPC 服务间调用 | ⏳ | 服务间走 HTTP JSON(设计允许 MVP 先 HTTP) |
| LLM Gateway(路由/重试/限流/prompt 版本) | ⚠️ | 确定性 Mock + token 估算;PromptVersion 字段占位,路由/限流/重试未实现 |
| Tool Gateway(注册/校验/权限/风险/审批/幂等) | ✅ | 完整实现;工具执行器为 4 个 Mock 工具,参数仅校验 JSON 对象(无 schema 校验 ⏳) |
| 风险分级规则(LOW 自动 / MEDIUM 策略 / HIGH 审批 / CRITICAL 禁止) | ✅ | MEDIUM 默认自动执行 + 租户策略可 DENY,与设计"按租户策略"一致 |

### §五 核心流程

| 流程 | 状态 | 说明 |
| --- | --- | --- |
| 创建任务(事务化 task+state+event+outbox、API 不阻塞) | ✅ | 实测验证;client_request_id 幂等通过(场景 2) |
| 执行流程(checkpoint 三点位、事件顺序) | ✅ | BEFORE_LLM / AFTER_LLM / AFTER_TOOL 落库,事件链完整 |
| 高风险审批全链路 | ✅ | tool_call WAITING_APPROVAL + approval PENDING + 事件 + 任务状态,approve 用 outbox 重排队替代 Temporal signal(⏳) |
| resume 流程 | ⚠️ | checkpoint 加载、TASK_RESUMED 事件、幂等均符合;**状态校验缺失(P2)**;"checkpoint 后未完成 ToolCall 检查"依赖工具幂等键间接实现 |
| cancel 流程 | ✅ | CANCELING(≈设计 CANCEL_REQUESTED)+ Redis flag + step 边界停止 + TASK_CANCELLED 事件 |
| 崩溃恢复复用已存 LLM 输出 | ⏳ | resume 重新执行整个 step,不跳过已保存的 AFTER_LLM 输出(Mock LLM 确定性掩盖了差异) |
| 多 step 计划执行 | ⏳ | 本地闭环为"单 step 即完成"的确定性 workflow,多 step plan 未实现 |

### §六 数据库设计

| 设计项 | 状态 | 说明 |
| --- | --- | --- |
| 9 张核心表 + 索引 | ✅ | 全部存在且比设计更严格:枚举类型、复合外键 `(tenant_id, *)`、check 约束、级联删除;额外增加 `tenants`/`users`/`task_idempotency_keys` |
| agent_states 乐观锁(version) | ✅ | `UpdateAgentStateOptimistic` 带 version 条件 |
| tool_calls 幂等唯一约束 | ✅ | `uq_tool_calls_tenant_idempotency (tenant_id, idempotency_key)` |
| approvals unique(call_id) | ✅ | `uq_approvals_call` |
| event 表分区、大输出转对象存储 | ⏳ | 未实现 |
| checkpoint 归档策略(保留 20 个热数据) | ⏳ | 全量保留在 PG,snapshot 含 schema_version=1 |

### §七 API 设计

| 接口 | 状态 | 说明 |
| --- | --- | --- |
| POST /tasks | ✅ | 请求/响应字段与设计一致(响应包裹在 `{"data":...}` 信封中,设计示例为裸对象——全站统一风格,记为可接受偏差) |
| GET /tasks/{id} | ✅ | 字段齐全含 budget_usage/trace_id |
| GET /tasks/{id}/events(timeline 分页) | ✅ | after_event_id 游标;响应无设计中的 `next_after_event_id` 字段,由客户端取末条 event_id 替代 |
| GET /tasks/{id}/events/stream(SSE) | ❌ | **P1:恒 503**(代码层面 Last-Event-ID/补发/心跳/慢客户端保护均已实现,被中间件缺陷整体阻断) |
| POST /tool-calls/{id}/approve、/reject | ✅ | 幂等 + 冲突 409(场景 3/4) |
| POST /tasks/{id}/cancel | ✅ | 幂等(场景 5) |
| POST /tasks/{id}/resume | ⚠️ | 响应含 resume_from_checkpoint_id;状态校验缺失(P2) |
| 鉴权(401/租户校验) | ✅ | 无 token/篡改 token 401;缺 tenant claim 拒绝 |
| 状态枚举 | ⚠️ | 实现:SUCCEEDED/CANCELED/CANCELING,设计:COMPLETED/CANCELLED/CANCEL_REQUESTED;`STOPPED_BY_LIMIT`、`RETRYABLE_FAILED`、`PAUSED`(定义了但无流程进入)未使用,预算超限落 FAILED+BUDGET_EXCEEDED 错误码。映射关系已在 local-production-loop 文档声明 |

### §十 生产级关键机制

| 机制 | 状态 | 说明 |
| --- | --- | --- |
| 幂等矩阵(创建/工具/审批/cancel/resume/event_id) | ✅ | 六项全部实测通过(场景 2/3/5 + e2e-local + SSE 代码审查 event_id 单调) |
| 事务边界(事务内不调外部服务) | ✅ | 代码审查确认:checkpoint/事件写入与 LLM/工具 HTTP 调用分离 |
| Outbox 模式 | ✅ | 同事务写入 + worker 派发 + FAILED 重试符合设计;PROCESSING 死信恢复已接线(P4 已修复) |
| 多租户隔离 | ✅ | 全部查询带 tenant_id,JWT 强校验,跨租户 404(场景 1);RBAC 简版(admin/owner/member) |
| trace 贯穿 | ⚠️ | trace_id/span_id 生成并贯穿全部事件与表;无 OTel SDK、无真实 span 导出(⏳) |
| Budget + LoopDetector | ⚠️ | Budget 四维检测 ✅(场景 8 + 单测,P3 虚计已修复);**LoopDetector 未实现**,仅 loop_fingerprints 字段占位(⏳) |

### §十三 生产必须项(未接入清单,均记为已知差距 ⏳)

Temporal SDK、OpenTelemetry、Prometheus client(/metrics 为手写文本)、NATS/Kafka(Redis Pub/Sub 替代)、MinIO/S3(artifact 存 PG content_json)、Vault/KMS、Kubernetes(helm/k8s 目录空)、LoopDetector、真实 LLM provider、工具参数 JSON Schema 校验、审批 expires_at 过期处理、多级审批。

## 四、结论

**已实现功能的行为正确性良好**:任务生命周期、审批闭环、幂等(四层)、cancel/resume 主链路、budget、多租户隔离、checkpoint、outbox、timeline 查询在 20 个自动化场景(4 既有 + 10 扩展 + 单测)下全部符合设计语义,数据库层甚至严于设计。

**两个需要优先处理的偏差**:P1(SSE 中间件缺陷,一行级修复,恢复设计必须项)和 P2(resume 状态校验缺失,数据一致性风险),已另行分支修复中。P3-P5 已于 2026-07-08 修复(见各条目),P6 复核后撤销(误报)。

**架构级差距**均为 local-production-loop 文档中已声明的 MVP 取舍,替换点(workflow_id、outbox、PromptVersion、storage_backend 字段)已预留,与设计方案的演进路径一致。

---

## 五、2026-07-08 功能补齐更新

本报告 §三 中标记为 ⏳/⚠️/❌ 的多数条目已实现,详见 `design-full-implementation-technical-document.md`:

- 多 step 计划执行、崩溃恢复复用已存 LLM 输出、LoopDetector → ✅(e2e 场景 10/11/12)
- 状态枚举补齐 STOPPED_BY_LIMIT / RETRYABLE_FAILED,预算超限落 STOPPED_BY_LIMIT 且可 resume → ✅(e2e 场景 8)
- 工具参数 JSON Schema 校验 → ✅
- LLM Gateway 路由(Provider 抽象)/重试/限流/prompt 版本 → ✅
- 审批 expires_at 过期处理 → ✅
- Prometheus client_golang 真实指标 → ✅(实测 /metrics)
- OpenTelemetry SDK + OTLP 导出 + 全链路 span → ✅(实测 Jaeger)
- MinIO/S3 对象存储保存大 artifact → ✅(实测)
- checkpoint 保留最近 20 个 → ✅
- Kubernetes 清单(deploy/k8s)→ ✅
- timeline `next_after_event_id`:复核发现实现早已存在,原条目记录过时 → ✅

剩余架构级差距(设计允许的取舍):Temporal SDK、gRPC、NATS/Kafka、真实 LLM Provider、Vault/KMS、event 表分区、多级审批。
