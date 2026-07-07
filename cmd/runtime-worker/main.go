package main

import (
	"log/slog"
	"os"

	"agent-runtime/internal/app"
	workerapp "agent-runtime/internal/app/worker"
)

func main() {
	if err := app.RunHTTPService("runtime-worker", ":8081", workerapp.RegisterRoutes); err != nil {
		slog.Error("runtime-worker 异常退出", "error", err)
		os.Exit(1)
	}
}
