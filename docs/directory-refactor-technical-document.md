# 目录重构技术文档

## 1. 重构依据

本次重构依据 `设计方案.md` 的“Go 目录结构设计”章节执行。该设计要求项目采用生产级清晰分层，不把代码长期堆放在简单的 `handler/service/model` 三层中。

核心目标是让目录结构提前匹配后续 20 周排期：服务边界、领域模型、用例、基础设施、传输层、可观测性、安全能力和公共工具都拥有稳定落点。

## 2. 重构结果总览

重构后的主要目录如下：

```text
cmd/                         服务进程入口
internal/app/                服务启动编排和服务专属路由
internal/domain/             领域模型边界
internal/usecase/            应用用例边界
internal/infra/              数据库、缓存、工作流、消息、对象存储、外部客户端
internal/transport/          HTTP、gRPC、SSE 传输层
internal/observability/      日志、指标、链路追踪
internal/security/           认证、授权、租户上下文
internal/config/             配置加载
pkg/                         可复用公共工具
db/migrations/               数据库迁移
db/queries/                  sqlc 查询定义
api/                         OpenAPI 和 proto
deploy/                      本地和生产部署配置
```

## 3. 代码迁移映射

| 原路径 | 新路径 | 说明 |
| --- | --- | --- |
| `internal/bootstrap` | `internal/app` | HTTP 服务启动编排属于应用层入口编排 |
| `cmd/*/main.go` 中的路由函数 | `internal/app/api`、`internal/app/worker`、`internal/app/toolgateway`、`internal/app/llmgateway` | 服务入口只保留启动逻辑，业务路由下沉到服务边界 |
| `internal/httpserver` | `internal/transport/http` | HTTP server、middleware、health 和 JSON response 归入传输层 |
| `internal/logging` | `internal/observability/logging` | 结构化日志属于可观测性能力 |
| `internal/apperrors` | `pkg/errors` | 统一错误码是跨层公共能力 |
| `internal/version` | `pkg/version` | 构建版本信息需要被健康检查和构建参数复用 |
| `internal/db` | `internal/infra/postgres/db` | sqlc 生成代码绑定 PostgreSQL 基础设施 |
| `internal/repository` | `internal/infra/postgres/repository` | Repository 依赖 PostgreSQL 查询实现，归入 PostgreSQL 基础设施边界 |
| `migrations` | `db/migrations` | 数据库迁移与 `db/queries` 放在同一数据库目录下 |
| `docker-compose.yml` | `deploy/docker-compose.yml` | 部署编排归入部署目录 |

## 4. 服务启动链路

以 API Service 为例，重构后的启动链路是：

1. `cmd/api-service/main.go` 调用 `app.RunHTTPService`。
2. `app.RunHTTPService` 加载配置、创建日志、创建 chi router、挂载 request_id、access log 和 recovery middleware。
3. 公共健康检查由 `internal/transport/http.RegisterHealthRoutes` 注册。
4. API 专属路由由 `internal/app/api.RegisterRoutes` 注册。
5. 最后由 `internal/transport/http.Run` 启动 `net/http` 服务并处理优雅关闭。

这样划分后，`cmd/*/main.go` 不再承载业务路由，后续任务创建、运行、审批、恢复等逻辑可以继续进入 `internal/app/*`、`internal/usecase/*` 和 `internal/domain/*`。

## 5. 数据库和部署路径调整

`sqlc.yaml` 已更新为：

```text
schema: db/migrations/000002_week2_core_models.up.sql
queries: db/queries
out: internal/infra/postgres/db
```

`scripts/migrate.sh` 默认读取 `db/migrations`。如果需要临时指定迁移目录，可以继续使用 `MIGRATIONS_DIR` 环境变量覆盖。

Docker Compose 文件移动到 `deploy/docker-compose.yml` 后，`Makefile` 和本地脚本都显式传入 `-f deploy/docker-compose.yml`，因此 `make docker-up`、`make docker-down` 和 `make docker-ps` 的使用方式不变。

## 6. 行为兼容性

本次重构不改变现有 HTTP 接口行为：

| 服务 | 端口 | 保留接口 |
| --- | ---: | --- |
| API Service | 8080 | `/healthz`、`/livez`、`/readyz`、`/api/v1/health`、`/api/v1/tasks`、`/api/v1/service-info` |
| Runtime Worker | 8081 | `/healthz`、`/livez`、`/readyz`、`/runtime/v1/status` |
| Tool Gateway | 8082 | `/healthz`、`/livez`、`/readyz`、`/tools/v1/catalog` |
| LLM Gateway | 8083 | `/healthz`、`/livez`、`/readyz`、`/llm/v1/status` |

统一错误响应、request_id、access log、panic recovery、health payload、graceful shutdown 逻辑保持不变，只改变所在目录。

## 7. 后续编码约束

后续新增代码应按职责选择目录：

1. 服务路由和服务级组装放入 `internal/app/<service>`。
2. 业务规则和核心状态模型放入 `internal/domain/<aggregate>`。
3. 任务创建、运行、恢复、取消、审批等业务流程放入 `internal/usecase/<usecase>`。
4. 数据库、Redis、Temporal、NATS、对象存储、LLM 和工具客户端实现放入 `internal/infra/*`。
5. HTTP、gRPC、SSE 入口协议适配放入 `internal/transport/*`。
6. 日志、指标、链路追踪放入 `internal/observability/*`。
7. 认证、授权、租户解析放入 `internal/security/*`。

如果后续需求需要改变这些边界，应先更新设计方案并确认后再编码。
