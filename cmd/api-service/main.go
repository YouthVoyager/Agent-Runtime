package main

import (
	"log/slog"
	"os"

	"agent-runtime/internal/app"
	apiapp "agent-runtime/internal/app/api"
)

// main 启动用户 API 服务入口。
func main() {
	if err := app.RunHTTPService("api-service", ":8080", apiapp.RegisterRoutes); err != nil {
		slog.Error("api-service 异常退出", "error", err)
		os.Exit(1)
	}
}
