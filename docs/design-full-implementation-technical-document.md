# 设计方案功能补齐技术文档

- 日期:2026-07-08
- 基线:`feature` 分支 `bc7ab43`(SSE/resume 修复)之后
- 目标:补齐《设计方案.md》要求但此前标记为"已知差距(⏳)"的功能,使代码实现覆盖设计方案的全部可落地功能项

## 1. 本次实现的功能总览

| # | 设计出处 | 功能 | 实现位置 |
| --- | --- | --- | --- |
| 1 | §5.2 / §12 难点一 | 多 step 计划执行(step 边界检查取消/预算/循环) | `internal/usecase/runtimeworker/service.go` |
| 2 | §12 难点一测试表 | 崩溃恢复复用已保存 LLM 输出 | 同上 + AFTER_LLM checkpoint 快照 |
| 3 | §9.6 / §十四亮点 6 | LoopDetector 循环检测 | `internal/domain/loopdetect/` |
| 4 | §9.1 | `STOPPED_BY_LIMIT` / `RETRYABLE_FAILED` 状态,预算超限落 STOPPED_BY_LIMIT | 迁移 `000004` + worker |
| 5 | §5.4 | resume 状态白名单(FAILED/PAUSED/STOPPED_BY_LIMIT) | `internal/usecase/taskapi/control.go` |
| 6 | §4.4 / §12 难点二 | 工具参数 JSON Schema 校验 | `internal/domain/toolcall/schema.go` |
| 7 | §4.3 | LLM Gateway:Provider 抽象、重试、按租户限流、prompt 版本管理 | `internal/usecase/llmgateway/` |
| 8 | §6.7 approvals.expires_at | 审批过期自动处理(EXPIRED) | worker `expireApprovals` |
| 9 | §2 必须项 | Prometheus client_golang 真实指标 | `internal/observability/metrics/` |
| 10 | §2 必须项 / §10.5 | OpenTelemetry SDK + OTLP 导出 + 跨服务传播 | `internal/observability/tracing/` |
| 11 | §2 必须项 / §6.3 | MinIO/S3 对象存储保存大 artifact | `internal/infra/objectstore/` |
| 12 | §6.6 | checkpoint 保留最近 20 个热数据 | `PruneCheckpoints` |
| 13 | §11.1 | Kubernetes 部署清单 | `deploy/k8s/` |

## 2. 数据库变更(迁移 000004)

```sql
alter type task_status add value if not exists 'STOPPED_BY_LIMIT';
alter type task_status add value if not exists 'RETRYABLE_FAILED';
```

- PostgreSQL 枚举值不可删除,down 迁移为 no-op(保留新值不影响旧代码)。
- `sqlc.yaml` schema 挂上 000004,重新生成后获得 `db.TaskStatusSTOPPEDBYLIMIT` 等常量。
- 新增查询:`MarkAgentTaskStopped`(落 STOPPED_BY_LIMIT + 错误码)、`GetLatestCheckpointByReason`(按 reason 查最新 checkpoint)、`PruneCheckpoints`(按任务保留最近 N 个)、`ListExpiredPendingApprovals` / `ExpireApproval`(审批过期)。
- 状态命名沿用既有映射(SUCCEEDED≈COMPLETED、CANCELED≈CANCELLED、CANCELING≈CANCEL_REQUESTED),见 local-production-loop 文档 §2;本次只补齐设计中缺失的两个功能性状态。

## 3. 多 step 执行循环(worker 重构)

原实现"单 step 即完成"。现在 `runTask` 变成循环:

```text
lockQueuedTask (QUEUED→RUNNING 原子锁)
loop (≤ maxStepsPerRun=100 安全上限):
    检查 Redis cancel flag  → finishCanceledTask
    budget.check           → 超限落 STOPPED_BY_LIMIT + TASK_STOPPED_BY_LIMIT 事件
    加载 AgentState
    runStep:
        STEP_STARTED 事件 + BEFORE_LLM checkpoint
        loadReusableLLMResponse:若当前 step 已有 AFTER_LLM checkpoint,复用其中 llm_response,跳过 LLM 调用
        否则调用 LLM Gateway → AFTER_LLM checkpoint(快照含 llm_response)
        LoopDetector:step 指纹连续重复 ≥3 次 → STOPPED_BY_LIMIT(LOOP_DETECTED)
        如有 tool call → Tool Gateway(幂等键 task+step+tool+argsHash)
            WAITING_APPROVAL → 返回,workflow 挂起
            FAILED → failTask
        commitStep:乐观锁更新 state(step+1、指纹历史)、累计预算、AFTER_TOOL checkpoint、STEP_COMPLETED
        IsFinal → SUCCEEDED + TASK_COMPLETED
```

关键点:

- **LLM 输出复用**:`AFTER_LLM` checkpoint 快照新增 `llm_response` 字段。恢复执行同一 step 时(worker 崩溃、审批通过重排队、resume)直接复用,不重复调用 LLM、不重复计费;timeline 写 `LLM_CALL_COMPLETED{reused_from_checkpoint:true}`。
- **预算语义**:每个 step 完成后立即累计 used_steps/tokens/tool_calls/cost 并检查;超限任务状态为 `STOPPED_BY_LIMIT`(此前是 FAILED),提高预算后可 resume 继续(e2e 场景 8 实测)。
- **审批链路不变**:step 未提交前挂起,审批通过后从同一 step 复用 LLM 输出 + 幂等键命中已审批 PENDING 工具调用继续执行。

## 4. LoopDetector(`internal/domain/loopdetect`)

- `BuildFingerprint(toolName, argumentsHash, content)`:SHA-256 截断指纹,规范化大小写与空白。
- `Detect(history, fp, threshold)`:历史尾部连续相同指纹次数 +1 达到阈值(默认 3)即判定循环。
- 指纹历史持久化在 `agent_states.loop_fingerprints`(上限 50 条),随 step 提交写入,崩溃后可恢复。
- Mock LLM 对含"循环/loop"的目标永远返回相同 step,用于端到端演示(e2e 场景 11)。

## 5. 工具参数 JSON Schema 校验

- `Definition` 增加 `Schema ArgumentSchema`(JSON Schema 子集:`type/properties/required/additionalProperties/maxLength`),四个内置工具全部定义 schema,`/tools/v1/catalog` 一并暴露。
- `ValidateArguments` 在 Tool Gateway `Call` 入口执行:缺必填、类型不符、未定义字段、超长均返回 400 `INVALID_ARGUMENT`,不落 tool_call 记录。
- 采用零依赖手写校验器与仓库风格一致(JWT/metrics 同为手写),结构与标准 JSON Schema 对齐,将来可无缝替换完整实现。

## 6. LLM Gateway 生产化

- **Provider 接口**:`Chat(ctx, req, prompt)`,Mock 实现 `mockProvider` 保持确定性多 step 计划;接真实模型时新增 Provider 即可。
- **重试**:`MarkRetryable/IsRetryable` 错误分类,可重试错误按 100ms 指数退避重试(默认 2 次),不可重试立即失败。
- **限流**:按租户令牌桶(默认 600 次/分钟),超限返回 429 `RATE_LIMITED`(新增错误码);`ChatRequest` 增加 `tenant_id` 字段由 worker 填充。
- **超时**:单次 provider 调用默认 60s 超时。
- **Prompt 版本**:`promptRegistry` 版本化管理,响应回填 `prompt_version`,历史版本可按版本号回放。

## 7. 审批过期处理

- 创建审批时写入 `expires_at = now() + 24h`。
- worker 每 tick 扫描 `ListExpiredPendingApprovals`:条件更新(`status='PENDING'`)置 EXPIRED 与人工决策互斥;同步 tool_call → FAILED(`TOOL_APPROVAL_EXPIRED`)+ approval_status EXPIRED;仍在 WAITING_APPROVAL 的任务落 FAILED;写 `TOOL_APPROVAL_DECIDED{decision:EXPIRED}` 事件。

## 8. 可观测性

### Prometheus(`internal/observability/metrics`)

引入 `prometheus/client_golang`,`/metrics` 由 promhttp 提供(含 Go runtime 指标),业务指标:

```text
stableagent_service_up{service,env}
stableagent_http_requests_total{service,method,status} + duration histogram
stableagent_tasks_finished_total{status}      # SUCCEEDED/FAILED/CANCELED/STOPPED_BY_LIMIT
stableagent_agent_steps_total
stableagent_llm_call_duration_seconds{result} / stableagent_llm_tokens_total
stableagent_tool_call_duration_seconds{tool,result}
stableagent_outbox_dispatch_total{result}
stableagent_approvals_expired_total
```

HTTP 侧由新中间件 `WithMetrics` 统一记录;k8s 清单带 `prometheus.io/scrape` 注解。

### OpenTelemetry(`internal/observability/tracing`)

- `Setup`:配置 `OTEL_EXPORTER_OTLP_ENDPOINT` 时启用 OTLP HTTP exporter + BatchProcessor;未配置为 no-op,业务零开销。
- 中间件 `WithTracing`:提取 W3C traceparent,按请求创建 server span;worker `postJSON` 注入 traceparent,实现 api→worker→llm/tool gateway 全链路串联。
- 业务 span 覆盖设计 §10.5 清单:`task.create`、`agent.step`、`llm.call`、`tool.call`、`checkpoint.create`、`budget.check`、`loop.detect`、`approval.decide`、`artifact.save`,统一携带 `stableagent.task_id/tenant_id` 属性。
- 本地 compose 已把 OTLP 指向 Jaeger(4318),Jaeger UI 实测可见完整 span 链。
- 注意:semconv 版本必须与 SDK `resource.Default()` 的 schema 一致(v1.41.0),否则 `resource.Merge` 报 schema 冲突——已加单测 `TestSetupWithEndpoint` 防回归。

## 9. 对象存储(MinIO/S3)

- `internal/infra/objectstore`:`Store` 接口 + minio-go 实现,启动时确保 bucket 存在;`MINIO_ENDPOINT` 为空则关闭。
- Tool Gateway `writeArtifact`:内容 > `ARTIFACT_INLINE_MAX_BYTES`(默认 4096)时上传 `tenant/task/artifact.json` 到 MinIO,DB 只存 `storage_backend='S3'` + `storage_key`;小内容仍内联 `content_json`;对象存储不可用自动回退内联,任务不失败。两种路径都写 `checksum_sha256`。
- compose 阈值参数化(`ARTIFACT_INLINE_MAX_BYTES`);实测阈值 1 字节时 artifact 落 MinIO 且 DB 记录正确。

## 10. 其他

- **checkpoint 保留策略**:每次创建 checkpoint 后 `PruneCheckpoints` 只保留最近 20 个(设计 §6.6)。
- **timeline 游标**:`GET /tasks/{id}/events` 响应的 `next_after_event_id` 经复核已在 eventapi 实现(符合度报告原条目过时)。
- **k8s 清单**(`deploy/k8s/`):四服务 Deployment/Service(readiness `/readyz`、liveness `/healthz`、非 root、滚动发布)、ConfigMap、Secret 示例、runtime-worker HPA、README。
- **web-ui**:timeline 新增 `TASK_STOPPED_BY_LIMIT`(超限停止)、`TASK_RESUMED`(任务恢复)事件文案。

## 11. 测试与验证结果

| 检查 | 结果 |
| --- | --- |
| `go vet ./...` + `go test ./...`(17 个测试包) | 全部通过 |
| 新增单测 | loopdetect(4)、toolcall schema(7)、llmgateway 重试/限流/多 step 计划(8)、tracing Setup(2) |
| `make build` / Docker 镜像构建 | 通过 |
| `make migrate-up`(000004) | 通过,幂等 |
| `make e2e-local`(4 场景) | 通过 |
| `make e2e-extended`(12 场景,新增场景 11 循环检测、场景 12 多 step 计划,场景 8 改为超限停止+恢复) | 12/12 通过 |
| /metrics 实测 | 任务终态、step、LLM/tool 时延、outbox 指标均有真实数据 |
| Jaeger 实测 | `agent.step/llm.call/tool.call/checkpoint.create/budget.check/loop.detect/artifact.save` + 三个服务的 server span 同链路可见 |
| MinIO 实测 | 阈值 1 字节时 artifact 写入 bucket,DB 记录 storage_backend=S3 |

## 12. 仍属架构级差距(设计允许的取舍)

- Temporal SDK:设计可选项明确"学习型项目可自研状态机";本地 workflow 已固定 `workflow_id=task_id` + outbox 替换点。
- gRPC:MVP 走 HTTP JSON(设计允许)。
- NATS/Kafka:Redis Pub/Sub 承担事件扇出(消息队列不在设计 §13 必须项列表)。
- 真实 LLM Provider、Vault/KMS、event 表分区、多级审批:接口/字段已预留。
