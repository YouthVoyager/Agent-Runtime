# 开发规范

## 1. Go 代码规范

1. Go 版本使用 `go1.26.x`。
2. 所有 Go 代码必须通过 `gofmt`，提交前运行 `make fmt`。
3. 包名使用小写单词，避免下划线和复数泛化命名。
4. 公共能力放在 `internal/` 下，避免未稳定 API 被外部项目依赖。
5. 服务入口只放启动编排，业务逻辑必须下沉到 `internal/` 包。
6. 所有 HTTP 请求必须经过 request_id、access log 和 recovery middleware。
7. 所有错误返回必须使用统一错误码结构，不直接返回散乱字符串。
8. 默认注释使用中文，复杂逻辑必须解释业务原因，不写无意义注释。

## 2. 目录规范

```text
cmd/
  api-service/       用户 API 服务入口
  runtime-worker/    Agent Runtime 后台进程入口
  tool-gateway/      工具调用网关入口
  llm-gateway/       LLM 网关入口
internal/
  apperrors/         统一错误码
  bootstrap/         服务启动编排
  config/            配置加载
  httpserver/        HTTP server、middleware、health
  logging/           结构化日志
  version/           构建版本信息
config/              本地配置样例
docs/                项目文档
migrations/          数据库迁移 SQL
scripts/             本地开发脚本
```

## 3. 分支规范

1. 主干分支为 `main`。
2. 功能分支使用 `feature/<scope>`。
3. 修复分支使用 `fix/<scope>`。
4. AI 自动实现分支如需要创建，默认使用 `codex/<scope>`。
5. 禁止在未确认的情况下回退他人的未提交改动。

## 4. Commit 规范

提交信息使用以下格式：

```text
<type>: <summary>
```

常用类型：

| 类型 | 说明 |
| --- | --- |
| `feat` | 新功能 |
| `fix` | 修复问题 |
| `docs` | 文档变更 |
| `chore` | 工程配置、脚本、依赖 |
| `test` | 测试 |
| `refactor` | 不改变行为的重构 |

AI 自动提交必须明确标注：

```text
ai自动提交: <summary>
```

## 5. Lint 和测试规范

1. 每次提交前运行 `make test`。
2. 有 `golangci-lint` 时运行 `make lint`。
3. 第 1 周至少覆盖配置加载、错误码、request_id middleware 的基础测试。
4. 后续涉及数据库、任务状态机、审批和 checkpoint 时，需要补充集成测试。

## 6. HTTP 规范

1. 每个请求必须带 `X-Request-ID`，如果调用方没有传入，服务端自动生成。
2. 日志必须包含 `service`、`env`、`request_id`、`method`、`path`、`status`、`duration_ms`。
3. 健康检查统一暴露 `/healthz`、`/livez`、`/readyz`。
4. 业务错误统一返回：

```json
{
  "error": {
    "code": "INVALID_ARGUMENT",
    "message": "参数错误"
  }
}
```

