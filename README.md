# StableAgent：生产级通用 Agent 执行平台

StableAgent 是一个以稳定性为核心的生产级通用 Agent 平台，支持长任务可恢复执行、工具调用安全治理、人工审批、执行 timeline、checkpoint、cancel/resume 和完整可观测 trace。

当前仓库是 StableAgent 的 Go monorepo，已按设计方案完成目录分层：四个服务入口保留在 `cmd/`，服务路由下沉到 `internal/app/`，传输层、可观测性、基础设施和公共工具分别归入对应目录。

## 服务边界

| 服务 | 目录 | 默认端口 | 当前职责 |
| --- | --- | ---: | --- |
| API Service | `cmd/api-service` | 8080 | 用户 API 入口，承载任务创建、查询、timeline、SSE 和 React 静态前端 |
| Runtime Worker | `cmd/runtime-worker` | 8081 | 后台执行进程，扫描 outbox，推进本地 workflow、checkpoint、cancel、resume |
| Tool Gateway | `cmd/tool-gateway` | 8082（Docker 宿主 18082） | 工具调用网关，提供工具目录、风险识别、幂等、审批和 Mock 工具执行 |
| LLM Gateway | `cmd/llm-gateway` | 8083 | 模型调用统一入口，当前提供可替换 Mock Provider、token 估算和 prompt version |

## 快速启动

本地直接启动单个服务：

```bash
npm --prefix web-ui run build
go run ./cmd/api-service
```

启动后可访问 `http://localhost:8080/` 查看 Event Timeline 前端原型。API Service 默认托管 `web-ui/dist`，可通过 `API_SERVICE_WEB_STATIC_DIR` 覆盖。

Docker 一键启动完整本地环境：

```bash
make docker-up
make health
```

Docker 镜像会在构建阶段生成并内置 `web-ui/dist`，因此通过 Compose 启动后可以直接访问 `http://localhost:8080/`。

生成本地 admin JWT：

```bash
make jwt
```

执行本地端到端闭环验证：

```bash
make e2e-local
```

停止本地环境：

```bash
make docker-down
```

## Health Check

```bash
curl http://localhost:8080/healthz
curl http://localhost:8081/healthz
curl http://localhost:18082/healthz
curl http://localhost:8083/healthz
```

所有服务都会返回统一 JSON，包含 `status`、`service`、`env`、`version`、`commit`、`build_time` 和 `uptime`。

## 常用命令

| 命令 | 说明 |
| --- | --- |
| `make fmt` | 格式化 Go 代码 |
| `make test` | 运行单元测试 |
| `make lint` | 运行 `golangci-lint` |
| `make build` | 构建四个服务到 `bin/` |
| `make docker-up` | 构建并启动本地 Docker Compose 环境 |
| `make docker-ps` | 查看 Compose 服务状态 |
| `make migrate-up` | 执行数据库 up migration |
| `make migrate-down` | 回滚最近一次 migration |
| `make health` | 检查四个服务 health check |
| `make jwt` | 生成本地 `tenant_local/admin_local` HS256 JWT |
| `make e2e-local` | 验证普通任务、高风险审批、cancel 和 resume 闭环 |
| `npm --prefix web-ui run build` | 构建 React timeline 静态文件 |

## 本地基础设施

`deploy/docker-compose.yml` 会启动：

| 组件 | 端口 | 用途 |
| --- | ---: | --- |
| PostgreSQL | 5432 | 业务主库 |
| Redis | 6379 | 缓存、锁、cancel flag、限流预留 |
| Temporal | 7233 | 长任务工作流引擎 |
| Temporal UI | 8088 | 工作流调试 UI |
| MinIO | 9000 / 9001 | S3 兼容对象存储 |
| Jaeger | 16686 / 4317 / 4318 / 14268 | Trace 后端 |

## 文档

| 文档 | 内容 |
| --- | --- |
| `docs/development-standards.md` | 代码规范、分支规范、commit 规范 |
| `docs/local-development.md` | 本地启动、迁移、调试、排障说明 |
| `docs/api-service-docker-static-web-technical-document.md` | API Service Docker 静态前端修复说明 |
| `docs/directory-refactor-technical-document.md` | 本次目录重构技术说明 |
| `docs/week-1-technical-document.md` | 第一周技术实现说明 |
| `docs/week-2-database-design.md` | 第二周数据库设计说明 |
| `docs/week-3-task-api-technical-document.md` | 第三周 Task API 技术说明 |
| `docs/week-4-event-timeline-technical-document.md` | 第四周 AgentEvent、Timeline、SSE 和前端原型技术说明 |
| `docs/local-production-loop-technical-document.md` | 本地生产闭环、Mock LLM/Tool、审批、cancel/resume 和验证说明 |
| `docs/stableagent-rename-technical-document.md` | StableAgent 项目改名技术说明 |
