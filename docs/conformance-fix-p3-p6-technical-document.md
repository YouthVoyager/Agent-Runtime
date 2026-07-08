# 符合度问题 P3-P6 修复技术文档

- 日期:2026-07-08
- 关联报告:`docs/design-conformance-report.md`(问题编号沿用该报告)
- 范围:P3、P4、P5 修复,P6 复核撤销;P1、P2 在另一分支处理,不在本次范围。

## 1. P3 预算 tool_calls 虚计

**问题**:`internal/usecase/runtimeworker/service.go` 的 `incrementBudgetUsage` 无条件 `UsedToolCalls++`,LLM 直接返回终态(无工具调用)的 step 也被计入,max_tool_calls 预算提前耗尽。

**修复**:
- `completeTask` 增加 `toolCalled bool` 参数,两个调用点分别传 `false`(`llmResp.ToolCall == nil || llmResp.IsFinal` 分支)和 `true`(工具执行成功分支);
- `incrementBudgetUsage` 仅在 `toolCalled` 为真时累计 `UsedToolCalls`,step/token/cost 照常累计;
- 新增单测 `TestIncrementBudgetUsage`(`internal/usecase/runtimeworker/budget_test.go`)覆盖两个分支。

## 2. P4 outbox PROCESSING 死信恢复未接线

**问题**:`ResetStaleTaskOutboxProcessing` 查询在 `db/queries/outbox.sql` 中已定义但无调用方;worker 在 `MarkTaskOutboxProcessing` 后崩溃,消息永久停留 PROCESSING。

**深层根因(本次一并修复)**:`sqlc.yaml` 的 `schema` 只包含 `000002` 迁移,缺 `000003`(`task_idempotency_keys` 表),导致 `make sqlc-generate` 直接报错、生成代码长期无法再生:
- `ResetStaleTaskOutboxProcessing` 从未出现在生成代码中;
- `internal/infra/postgres/db/events_ext.go` 是绕过生成失败手写的补丁文件。

**修复**:
1. `sqlc.yaml` schema 补齐 3 个 up 迁移(000001/000002/000003),`make sqlc-generate` 恢复可用;
2. 删除手写补丁 `events_ext.go`(与重新生成的 `events.sql.go` 重复声明);
3. `runtimeworker.dispatchOutbox` 在派发前调用 `ResetStaleTaskOutboxProcessing`,阈值常量 `outboxStaleAfter = 30s`(轮询间隔 1s、失败重试间隔 5s,30s 足以判定死信);重置后的消息进入既有 `FAILED + next_retry_at` 重试通道,复用 `retry_count+1` 逻辑。

**注意**:重新生成使 `internal/infra/postgres/db/` 下多个文件出现 diff(字段顺序、新增 `TaskIdempotencyKey` 模型与查询),均为生成器输出,`go build ./...` 与全量测试通过。

## 3. P5 verify-services.sh 端口冲突

**问题**:脚本用 18082 启动本地 tool-gateway 二进制,与 docker-compose 中 tool-gateway 的对外映射端口相同;Docker 环境运行时二进制绑定失败,健康检查却命中容器,冒烟结果失真。

**修复**:tool-gateway 备用端口改为 18084(其余 18080/18081/18083 与 compose 无冲突),脚本内注释说明原因。

## 4. P6 撤销(误报)

复核确认 `web-ui/src/app.js` 是 React 实现:`import React from 'https://esm.sh/react@19.2.3'`,浏览器运行时经 ESM CDN 加载,因此 `package.json` 零 npm 依赖。week-4 技术文档第 233 行对该方案有准确说明,文档与实现一致。符合度报告中 P6 条目已标注撤销。

## 5. 验证结果

| 检查 | 结果 |
| --- | --- |
| `go vet ./...` + `make test` | 全部通过(13 个测试包) |
| `bash scripts/verify-services.sh` | 4 个二进制健康检查通过(新端口) |
| `make docker-up`(重建镜像)+ `make health` | 通过 |
| `make e2e-local` | 4/4 场景通过 |
| `make e2e-extended` | 9/10 通过;场景 7(SSE)为 P1 已知失败,待另一分支修复后转绿 |
