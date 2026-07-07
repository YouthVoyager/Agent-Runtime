package main

import (
	"log/slog"
	"os"

	"agent-runtime/internal/app"
	llmgatewayapp "agent-runtime/internal/app/llmgateway"
)

// main 启动 LLM 网关服务入口。
func main() {
	if err := app.RunHTTPService("llm-gateway", ":8083", llmgatewayapp.RegisterRoutes); err != nil {
		slog.Error("llm-gateway 异常退出", "error", err)
		os.Exit(1)
	}
}
