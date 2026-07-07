# 本地开发与启动文档

## 1. 环境要求

| 工具 | 版本建议 | 用途 |
| --- | --- | --- |
| Go | `1.26.x` | 编译和运行服务 |
| Docker Desktop | 当前稳定版 | 本地基础设施 |
| Docker Compose | v2 | 一键启动本地环境 |
| golangci-lint | 当前稳定版 | 代码静态检查 |

## 2. 配置文件

默认配置来自环境变量。可以复制样例文件：

```bash
cp config/local.env.example config/local.env
```

服务启动时会读取 `APP_CONFIG_FILE` 指定的文件；如果没有设置，则尝试读取 `config/local.env`。文件不存在不会导致启动失败。

配置优先级：

1. 已存在的系统环境变量。
2. `APP_CONFIG_FILE` 或 `config/local.env` 中的变量。
3. 代码内置默认值。

服务级配置使用服务名前缀，例如：

```text
API_SERVICE_HTTP_ADDR=:8080
RUNTIME_WORKER_HTTP_ADDR=:8081
TOOL_GATEWAY_HTTP_ADDR=:8082
LLM_GATEWAY_HTTP_ADDR=:8083
```

## 3. 本地直接运行

```bash
go run ./cmd/api-service
go run ./cmd/runtime-worker
go run ./cmd/tool-gateway
go run ./cmd/llm-gateway
```

也可以使用脚本：

```bash
./scripts/run-service.sh api-service
```

## 4. Docker Compose 一键启动

```bash
make docker-up
```

该命令会构建四个 Go 服务镜像，并启动 PostgreSQL、Redis、Temporal、Temporal UI、MinIO、Jaeger。

查看状态：

```bash
make docker-ps
```

停止环境：

```bash
make docker-down
```

## 5. 数据库迁移

第 1 周提供轻量 migration 脚本，基于 Compose 中的 PostgreSQL 容器执行 SQL 文件。

执行迁移：

```bash
make migrate-up
```

查看迁移状态：

```bash
make migrate-status
```

回滚最近一次迁移：

```bash
make migrate-down
```

迁移文件约定：

```text
migrations/000001_xxx.up.sql
migrations/000001_xxx.down.sql
```

脚本会维护 `schema_migrations` 表，用于记录已经执行过的版本。

## 6. Health Check 验证

启动后执行：

```bash
make health
```

等价于检查：

```bash
curl http://localhost:8080/healthz
curl http://localhost:8081/healthz
curl http://localhost:8082/healthz
curl http://localhost:8083/healthz
```

## 7. 常见问题

### 端口被占用

修改 `config/local.env` 或 Compose 中对应服务的端口映射。例如 API Service 默认使用 `8080`。

### migration 连接失败

先确认 PostgreSQL 容器已启动：

```bash
docker compose ps postgres
```

然后再执行：

```bash
make migrate-up
```

### lint 命令不存在

`make lint` 依赖本机安装 `golangci-lint`。没有安装时，不影响 `make fmt`、`make test`、`make build`。

