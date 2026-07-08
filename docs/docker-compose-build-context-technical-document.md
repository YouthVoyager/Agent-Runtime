# Docker Compose 构建上下文修复技术文档

## 1. 问题现象

执行以下命令时，应用服务镜像构建失败：

```bash
docker compose -f deploy/docker-compose.yml up -d
```

错误信息为：

```text
failed to solve: failed to read dockerfile: open Dockerfile: no such file or directory
```

## 2. 根因分析

`deploy/docker-compose.yml` 位于 `deploy/` 目录。Docker Compose 解析相对路径时，会以 Compose 文件所在目录作为基准。因此原配置中的：

```yaml
x-service-build: &service-build
  context: .
  dockerfile: Dockerfile
```

实际会把 build context 解析为 `deploy/` 目录，并尝试读取 `deploy/Dockerfile`。项目真实的 `Dockerfile`、`go.mod`、`cmd/`、`internal/` 都在仓库根目录，所以构建上下文不完整，最终导致 Dockerfile 不存在。

## 3. 修复方案

将应用服务公共构建配置改为：

```yaml
x-service-build: &service-build
  context: ..
  dockerfile: Dockerfile
```

`context: ..` 表示构建上下文回到仓库根目录。`dockerfile: Dockerfile` 在该上下文内继续指向根目录下的 Dockerfile。

## 4. 影响范围

本次只调整 Compose 构建路径，不改变服务端口、环境变量、镜像构建参数或容器启动命令。以下命令都会使用修复后的构建上下文：

```bash
make docker-up
./scripts/dev-up.sh
docker compose -f deploy/docker-compose.yml up -d --build
```

## 5. 验证方式

1. 使用 `docker compose -f deploy/docker-compose.yml config` 检查 Compose 配置可解析。
2. 使用 `docker compose -f deploy/docker-compose.yml build api-service` 验证至少一个 Go 服务可以从仓库根目录完成镜像构建。
3. 使用 `go test ./...` 确认本次部署配置修复没有影响 Go 代码编译和测试。
