package main

import (
	"log/slog"
	"os"

	"stableagent/internal/app"
	toolgatewayapp "stableagent/internal/app/toolgateway"
)

// main 启动工具调用网关服务入口。
func main() {
	if err := app.RunHTTPService("tool-gateway", ":8082", toolgatewayapp.RegisterRoutes); err != nil {
		slog.Error("tool-gateway 异常退出", "error", err)
		os.Exit(1)
	}
}
