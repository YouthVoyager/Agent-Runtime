# Temporal 动态配置缺失修复技术文档

## 1. 问题现象

本地执行 Docker Compose 启动 Temporal 时，Temporal PostgreSQL schema 初始化成功，但 Temporal Server 随后启动失败。

关键错误信息如下：

```text
Unable to create dynamic config client.
unable to validate dynamic config:
dynamic config: config/dynamicconfig/development-sql.yaml:
stat config/dynamicconfig/development-sql.yaml: no such file or directory
```

## 2. 根因分析

`deploy/docker-compose.yml` 中 Temporal 服务通过 `DYNAMIC_CONFIG_FILE_PATH` 指定动态配置文件：

```yaml
DYNAMIC_CONFIG_FILE_PATH: config/dynamicconfig/development-sql.yaml
```

Temporal auto-setup 镜像启动时会读取该路径。如果容器内不存在这个文件，动态配置客户端初始化会失败，Temporal Server 也会随之退出。

本次错误不是 PostgreSQL 连接失败，也不是 Temporal schema 初始化失败。日志中已经出现 `UpdateSchemaTask done`，说明数据库初始化流程已完成；失败点发生在加载 Temporal Server 运行配置阶段。

## 3. 修复方案

本次修复保留原有设计方案中的 `Temporal + PostgreSQL + Docker Compose` 本地基础设施设计，只补齐缺失的动态配置文件和挂载关系。

新增文件：

```text
deploy/temporal/dynamicconfig/development-sql.yaml
```

文件内容使用空 YAML 映射：

```yaml
{}
```

该内容表示当前本地环境不额外覆盖 Temporal 动态配置，但仍向 Temporal 提供一个合法存在的动态配置文件。

同时在 Temporal 服务中新增只读挂载：

```yaml
volumes:
  - ./temporal/dynamicconfig:/etc/temporal/config/dynamicconfig:ro
```

因为 `deploy/docker-compose.yml` 位于 `deploy/` 目录，Compose 会以该文件所在目录解析相对路径，所以宿主机路径 `./temporal/dynamicconfig` 实际对应仓库内：

```text
deploy/temporal/dynamicconfig
```

容器内挂载目标为：

```text
/etc/temporal/config/dynamicconfig
```

Temporal auto-setup 在容器内加载 `config/dynamicconfig/development-sql.yaml` 时，即可读取到挂载后的文件。

## 4. 影响范围

本次变更只影响本地 Docker Compose 中 Temporal 服务的动态配置文件来源。

不改变以下内容：

1. Temporal 镜像版本。
2. Temporal PostgreSQL 连接参数。
3. 应用服务访问 Temporal 的地址。
4. 对外暴露端口。
5. 业务数据库、Redis、MinIO、Jaeger 等其他基础设施配置。

## 5. 验证方式

使用以下命令检查 Compose 配置是否可解析：

```bash
docker compose -f deploy/docker-compose.yml config
```

使用以下命令启动本地环境：

```bash
make docker-up
```

如果只验证 Temporal，可在 `deploy/` 目录执行：

```bash
docker compose up -d temporal-postgres temporal
```

Temporal 正常启动后，原先的 `stat config/dynamicconfig/development-sql.yaml: no such file or directory` 错误不应再出现。
