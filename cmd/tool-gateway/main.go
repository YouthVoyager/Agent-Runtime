package main

import (
	"log/slog"
	"os"

	"agent-runtime/internal/app"
	toolgatewayapp "agent-runtime/internal/app/toolgateway"
)

func main() {
	if err := app.RunHTTPService("tool-gateway", ":8082", toolgatewayapp.RegisterRoutes); err != nil {
		slog.Error("tool-gateway 异常退出", "error", err)
		os.Exit(1)
	}
}
