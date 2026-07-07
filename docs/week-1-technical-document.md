# 第一周技术文档

## 1. 本周目标

第一周目标是完成 Agent Runtime 的工程基础，不实现完整任务业务闭环。交付内容包括：

1. Go monorepo 工程结构。
2. 四个服务入口：`api-service`、`runtime-worker`、`tool-gateway`、`llm-gateway`。
3. 配置管理、结构化日志、统一错误码。
4. request_id middleware、health check、graceful shutdown。
5. Dockerfile、Docker Compose、本地基础设施。
6. migration 目录和本地迁移脚本。
7. Makefile、开发脚本、lint 配置。
8. README、启动文档、开发规范文档。

## 2. 为什么拆成四个服务

设计方案要求生产级形态至少保留以下服务边界：

| 服务 | 生产职责 | 第 1 周实现状态 |
| --- | --- | --- |
| API Service | 对外 API，任务创建、查询、审批、cancel、resume | 已有 HTTP 服务入口和占位路由 |
| Runtime Worker | 后台执行 Agent 主循环、checkpoint、cancel 检查 | 已有 HTTP 健康检查和状态路由 |
| Tool Gateway | 工具注册、风险识别、审批、幂等、审计 | 已有工具目录占位路由 |
| LLM Gateway | 模型路由、token 统计、限流、重试 | 已有服务入口和状态路由 |

这样做的原因是后续业务实现可以直接放入已有服务边界，不需要从单体项目中拆分，降低第 3 周到第 11 周的重构成本。

## 3. 代码结构说明

```text
cmd/
  api-service/main.go
  runtime-worker/main.go
  tool-gateway/main.go
  llm-gateway/main.go
internal/
  apperrors/
  bootstrap/
  config/
  httpserver/
  logging/
  version/
```

`cmd/*/main.go` 只负责声明服务名、默认端口和服务专属路由。公共启动逻辑放在 `internal/bootstrap`，避免四个服务重复写信号处理、日志、中间件和 HTTP server。

## 4. 配置管理

配置加载入口是 `internal/config.Load`。

加载顺序：

1. 读取系统环境变量。
2. 如果设置了 `APP_CONFIG_FILE`，读取该文件。
3. 如果没有设置 `APP_CONFIG_FILE`，尝试读取 `config/local.env`。
4. 对缺失配置使用代码默认值。

服务级变量优先于通用变量。例如 `API_SERVICE_HTTP_ADDR` 会覆盖通用 `HTTP_ADDR`。这样四个服务可以共享一套默认配置，同时又能独立调整端口、日志级别和超时时间。

当前核心配置包括：

| 配置 | 说明 |
| --- | --- |
| `APP_ENV` | 运行环境，默认 `local` |
| `LOG_LEVEL` | 日志级别，默认 `info` |
| `*_HTTP_ADDR` | 服务监听地址 |
| `DATABASE_URL` | PostgreSQL 连接串 |
| `REDIS_ADDR` | Redis 地址 |
| `TEMPORAL_ADDRESS` | Temporal 地址 |
| `MINIO_ENDPOINT` | MinIO 地址 |
| `JAEGER_ENDPOINT` | Jaeger 上报地址 |
| `SHUTDOWN_TIMEOUT` | 优雅关闭等待时间 |

## 5. 结构化日志

日志入口是 `internal/logging.New`，基于 Go 标准库 `log/slog` 的 JSON handler。

每条日志默认带有：

```text
service
env
```

HTTP access log 额外包含：

```text
request_id
method
path
status
duration_ms
remote_addr
```

使用 JSON 日志是为了后续接入 Loki、ELK 或其他日志系统时可以按字段检索，例如按 `request_id` 串联一次请求的全部日志。

## 6. 统一错误码

错误码包位于 `internal/apperrors`。

当前内置错误码：

| 错误码 | HTTP 状态 | 用途 |
| --- | ---: | --- |
| `INVALID_ARGUMENT` | 400 | 参数错误 |
| `UNAUTHORIZED` | 401 | 未认证 |
| `FORBIDDEN` | 403 | 无权限 |
| `NOT_FOUND` | 404 | 资源不存在 |
| `CONFLICT` | 409 | 资源冲突 |
| `NOT_IMPLEMENTED` | 501 | 功能尚未实现 |
| `SERVICE_UNAVAILABLE` | 503 | 依赖服务不可用 |
| `INTERNAL_ERROR` | 500 | 未归类系统错误 |

统一响应结构：

```json
{
  "error": {
    "code": "NOT_IMPLEMENTED",
    "message": "第 1 周仅完成服务骨架"
  }
}
```

## 7. Request ID Middleware

`internal/httpserver.WithRequestID` 负责处理请求 ID。

规则：

1. 如果请求头已有 `X-Request-ID`，服务端沿用该值。
2. 如果请求头没有，服务端生成 16 字节随机 ID。
3. ID 会写回响应头。
4. ID 会放入 request context，供日志和后续业务使用。

这样用户 API、Worker、Tool Gateway、LLM Gateway 后续互相调用时，可以用同一个 request_id 追踪跨服务链路。

## 8. Health Check

所有服务统一注册：

```text
/healthz
/livez
/readyz
```

当前三个端点都返回进程级健康状态。后续接入数据库、Redis、Temporal 后，可以把 `/readyz` 扩展成依赖检查，而 `/livez` 仍只表示进程存活。

示例响应：

```json
{
  "status": "ok",
  "service": "api-service",
  "env": "local",
  "version": "dev",
  "commit": "none",
  "build_time": "unknown",
  "uptime": "10s"
}
```

## 9. Graceful Shutdown

四个服务都通过 `signal.NotifyContext` 监听 `SIGINT` 和 `SIGTERM`。

关闭流程：

1. 收到退出信号。
2. 停止接受新连接。
3. 等待正在处理的请求完成。
4. 超过 `SHUTDOWN_TIMEOUT` 后返回错误。
5. 输出关闭日志。

这个机制是后续部署到 Docker、Kubernetes 时的基础。Worker 后续执行长任务时，也必须基于同样的 context 机制在安全边界停止，而不是强杀 goroutine。

## 10. Docker 本地环境

`docker-compose.yml` 包含应用服务和基础设施。

应用服务：

| 服务 | 容器端口 | 宿主机端口 |
| --- | ---: | ---: |
| api-service | 8080 | 8080 |
| runtime-worker | 8081 | 8081 |
| tool-gateway | 8082 | 8082 |
| llm-gateway | 8083 | 8083 |

基础设施：

| 组件 | 端口 | 当前用途 |
| --- | ---: | --- |
| PostgreSQL | 5432 | 业务主库 |
| Redis | 6379 | cancel flag、锁、缓存、限流预留 |
| Temporal | 7233 | 长任务工作流预留 |
| Temporal UI | 8088 | 本地查看 workflow |
| MinIO | 9000 / 9001 | artifact 对象存储预留 |
| Jaeger | 16686 / 4317 / 4318 / 14268 | trace 采集和查看 |

## 11. Migration 工具

第 1 周使用 `scripts/migrate.sh` 作为轻量迁移工具。

设计原因：

1. 当前还没有引入 PostgreSQL Go driver。
2. 使用 Compose 中的 `psql` 就能执行 SQL。
3. 可以先建立 migration 目录和版本表，后续再替换为 goose 或 golang-migrate。

脚本维护 `schema_migrations` 表，记录已经应用过的版本。文件命名规则：

```text
000001_week1_baseline.up.sql
000001_week1_baseline.down.sql
```

当前基线迁移只创建 `pgcrypto` 扩展，业务表会按第 2 周计划创建。

## 12. 验证方式

本周代码级验证：

```bash
make fmt
make test
make build
```

本地服务验证：

```bash
go run ./cmd/api-service
curl http://localhost:8080/healthz
```

Docker 环境验证：

```bash
make docker-up
make health
make migrate-up
```

`make lint` 已接入，但依赖本机安装 `golangci-lint`。

## 13. 后续扩展点

第 2 周可以直接在当前骨架上继续实现：

1. `migrations/` 中新增核心表。
2. `internal/repository` 中接入 `pgx` 和 `sqlc`。
3. `api-service` 中实现任务创建、查询和 timeline API。
4. `runtime-worker` 中实现可恢复执行主循环。
5. `tool-gateway` 中实现工具幂等、风险判断和审批创建。
6. `llm-gateway` 中实现模型调用抽象、超时、重试和 token 统计。

