# StableAgent 项目改名技术文档

## 1. 变更背景

本次变更依据 `设计方案.md` 的项目定位章节执行。新的项目名称为 `StableAgent：生产级通用 Agent 执行平台`，一句话介绍为：

```text
StableAgent 是一个以稳定性为核心的生产级通用 Agent 平台，支持长任务可恢复执行、工具调用安全治理、人工审批、执行 timeline、checkpoint、cancel/resume 和完整可观测 trace。
```

本次不改变核心业务模型，只统一项目品牌、工程模块名和本地运行标识。

## 2. 改名范围

### 2.1 产品文档

`README.md`、`设计方案.md`、`执行计划.md` 和各阶段技术文档中的旧项目名已替换为 `StableAgent`。README 首页直接展示完整项目名和一句话介绍，方便非项目人员第一眼理解项目定位。

### 2.2 Go 工程模块

`go.mod` 的 module 已从 `agent-runtime` 改为 `stableagent`，所有 Go 文件中的内部 import 路径同步替换为 `stableagent/...`。这样后续代码、构建产物和错误栈里的模块路径都与新项目名称一致。

`Dockerfile` 的 `-ldflags` 也同步改为 `stableagent/pkg/version`，保证容器构建时仍能正确注入 `Version`、`Commit` 和 `BuildTime`。

### 2.3 本地运行标识

本地 Docker Compose project name 改为 `stableagent`，默认 PostgreSQL 用户、密码、数据库名改为 `stableagent`。`config/local.env.example`、`internal/config/config.go` 和 `scripts/migrate.sh` 的默认连接信息已同步调整。

Redis AgentEvent Pub/Sub channel 从 `agent-runtime:events` 改为 `stableagent:events`，用于区分新项目下的 timeline 广播消息。

### 2.4 API 与调试集合

OpenAPI 标题改为 `StableAgent API`，描述中加入新的平台定位。Postman 集合文件改为 `api/postman/stableagent.postman_collection.json`，Bruno 集合目录改为 `api/bruno/stableagent`，集合显示名也同步改为 `StableAgent`。

### 2.5 Web UI

前端页面标题和侧边栏品牌名改为 `StableAgent`，副标题使用 `生产级通用 Agent 执行平台`。`web-ui/package.json` 的包名改为 `stableagent-web-ui`。

浏览器 JWT 缓存 key 改为 `stableagent_jwt`。为了减少升级影响，前端初始化时会先读取新 key，读取不到时再兼容旧 key `agent_runtime_jwt`，随后写回新 key 并移除旧 key。

## 3. 保留不变的内容

数据库业务表名仍保留 `agent_tasks`、`agent_states`、`agent_events` 等命名。这些名字表达的是 Agent 领域模型，不是旧项目品牌；直接改表名会扩大 migration 和数据兼容成本，且不影响 StableAgent 的产品命名。

服务入口目录仍保留 `cmd/api-service`、`cmd/runtime-worker`、`cmd/tool-gateway` 和 `cmd/llm-gateway`。这些是服务职责名，不是旧项目名。

当前工作区物理目录 `/Users/hao/Desktop/Agent Runtime` 未重命名，避免破坏 Codex 当前会话和本地路径引用。

## 4. 本地兼容注意事项

已有 Docker volume 中如果已经初始化过旧数据库用户和库名，直接切换到新默认值可能导致连接失败。可以选择保留旧连接串环境变量覆盖默认值，也可以清理本地 Compose volume 后重新初始化 StableAgent 本地环境。

应用内部 API 路径保持 `/api/v1/...` 不变，因此客户端不需要因为本次改名调整 API 路由。
