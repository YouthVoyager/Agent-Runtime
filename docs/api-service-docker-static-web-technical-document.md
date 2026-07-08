# API Service Docker 静态前端修复技术文档

## 1. 问题现象

使用 Docker Compose 启动本地环境后，`api-service` 日志出现：

```text
React 静态目录不存在，跳过前端路由注册
```

随后访问 `http://localhost:8080/` 返回 404，但 `api-service` 本身已经正常启动，健康检查接口仍可访问。

## 2. 设计依据

根据 `设计方案.md`，`api-service` 是用户 API 入口，负责承载任务创建、查询、timeline、SSE 和前端管理台访问。

根据 `docs/week-4-event-timeline-technical-document.md`，React Timeline 前端的构建产物为：

```text
web-ui/dist/index.html
web-ui/dist/assets/app.js
web-ui/dist/assets/styles.css
```

API Service 默认托管 `web-ui/dist`，并允许通过 `API_SERVICE_WEB_STATIC_DIR` 覆盖静态目录。

## 3. 根因分析

后端静态路由注册逻辑位于 `internal/app/api/static_handler.go`。

服务启动时会读取 `API_SERVICE_WEB_STATIC_DIR`，如果没有显式配置，则使用默认值 `web-ui/dist`。随后代码会通过 `os.Stat` 检查该目录是否存在：

1. 目录存在时，注册 `/` 和 `/*` 前端路由。
2. 目录不存在时，只记录日志并跳过前端路由注册。
3. 前端路由未注册时，访问 `/` 会落到默认 404。

原 Dockerfile 的最终镜像只复制了 Go 二进制：

```dockerfile
COPY --from=build /out/service /service
```

因此容器内没有 `web-ui/dist`，API Service 启动时就无法注册前端路由。

## 4. 修复方案

本次修复采用镜像内构建并携带静态产物的方式，避免依赖宿主机提前执行 `npm --prefix web-ui run build`。

Dockerfile 新增 `web-build` 阶段：

1. 使用 `node:22-alpine` 作为前端构建环境。
2. 只复制 `web-ui/package.json`、`web-ui/index.html`、`web-ui/scripts` 和 `web-ui/src`。
3. 执行 `npm run build` 生成 `web-ui/dist`。

最终运行镜像新增两项配置：

```dockerfile
ENV API_SERVICE_WEB_STATIC_DIR=/web-ui/dist
COPY --from=web-build /src/web-ui/dist /web-ui/dist
```

这样容器启动时，API Service 会直接检查 `/web-ui/dist`，并注册 React 静态文件路由。

为了避免所有服务镜像都携带前端产物，Dockerfile 将最终运行阶段拆成两个 target：

| target | 用途 | 是否包含前端静态文件 |
| --- | --- | --- |
| `service-runtime` | runtime-worker、tool-gateway、llm-gateway 等普通服务 | 否 |
| `api-runtime` | api-service | 是 |

`deploy/docker-compose.yml` 默认使用 `service-runtime`，只有 `api-service` 覆盖为 `api-runtime`。Compose 中也显式配置：

```yaml
API_SERVICE_WEB_STATIC_DIR: /web-ui/dist
```

该配置让 Compose 本地环境的静态目录来源更清晰，也避免未来镜像默认环境变量变化时影响本地启动。

## 5. 运行时请求链路

修复后的 Docker 运行链路如下：

```text
docker compose build api-service
  -> web-build 阶段生成 /src/web-ui/dist
  -> build 阶段生成 /out/service
  -> api-runtime 镜像复制 /service 和 /web-ui/dist

api-service 启动
  -> 读取 API_SERVICE_WEB_STATIC_DIR=/web-ui/dist
  -> os.Stat("/web-ui/dist") 成功
  -> 注册 / 和 /* 静态路由

浏览器访问 http://localhost:8080/
  -> 返回 /web-ui/dist/index.html
  -> 浏览器继续加载 /assets/app.js 和 /assets/styles.css
```

## 6. 影响范围

本次变更只影响 Docker 镜像构建和 Compose 本地环境配置。

对本地直接运行没有破坏性影响。本地直接执行 `go run ./cmd/api-service` 时，仍然可以继续使用默认目录 `web-ui/dist`，或者通过 `API_SERVICE_WEB_STATIC_DIR` 指定其他目录。

对 `runtime-worker`、`tool-gateway` 和 `llm-gateway` 的业务逻辑没有影响。这些服务不会读取 `API_SERVICE_WEB_STATIC_DIR`，也不会注册前端静态路由。

## 7. 验证方式

建议按以下顺序验证：

```bash
npm --prefix web-ui run build
go test ./...
docker compose -f deploy/docker-compose.yml build api-service
```

如果需要完整启动本地环境：

```bash
make docker-up
curl http://localhost:8080/
```

预期结果：

1. `api-service` 日志不再出现 `React 静态目录不存在`。
2. `curl http://localhost:8080/` 返回 HTML。
3. 浏览器访问 `http://localhost:8080/` 可以打开 Event Timeline 前端原型。

## 8. 排障说明

如果仍然返回 404，优先检查容器内静态目录：

```bash
docker compose -f deploy/docker-compose.yml exec api-service ls -la /web-ui/dist
```

如果目录不存在，说明镜像没有使用最新 Dockerfile 重新构建，需要执行：

```bash
docker compose -f deploy/docker-compose.yml build api-service
docker compose -f deploy/docker-compose.yml up -d api-service
```

如果目录存在但页面资源加载失败，继续检查：

```bash
docker compose -f deploy/docker-compose.yml exec api-service ls -la /web-ui/dist/assets
```

正常情况下应包含：

```text
app.js
styles.css
```
