# Agent Runtime

Agent Runtime 是一个生产级 Agent 长任务执行平台的 Go monorepo。当前完成第 1 周工程骨架：四个基础服务、统一配置、结构化日志、错误码、request_id middleware、health check、graceful shutdown、Docker 本地环境、migration 脚本、Makefile 和开发规范文档。

## 服务边界

| 服务 | 目录 | 默认端口 | 当前职责 |
| --- | --- | ---: | --- |
| API Service | `cmd/api-service` | 8080 | 用户 API 入口，后续承载任务创建、查询、审批、取消和恢复 |
| Runtime Worker | `cmd/runtime-worker` | 8081 | 后台执行进程，后续承载 step、checkpoint、cancel、resume |
| Tool Gateway | `cmd/tool-gateway` | 8082 | 工具调用网关，当前提供 MVP 工具目录占位 |
| LLM Gateway | `cmd/llm-gateway` | 8083 | 模型调用统一入口，后续承载路由、限流、重试和 token 统计 |

## 快速启动

本地直接启动单个服务：

```bash
go run ./cmd/api-service
```

Docker 一键启动完整本地环境：

```bash
make docker-up
make health
```

停止本地环境：

```bash
make docker-down
```

## Health Check

```bash
curl http://localhost:8080/healthz
curl http://localhost:8081/healthz
curl http://localhost:8082/healthz
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

## 本地基础设施

`docker-compose.yml` 会启动：

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
| `docs/week-1-technical-document.md` | 第一周技术实现说明 |

