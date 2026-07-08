package tracing

import (
	"context"
	"testing"
)

// TestSetupNoop 校验未配置 exporter 时 Setup 返回 no-op 且不报错。
func TestSetupNoop(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := Setup(context.Background(), "test-service", "test")
	if err != nil {
		t.Fatalf("no-op Setup 不应报错: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown 不应报错: %v", err)
	}
}

// TestSetupWithEndpoint 校验配置 OTLP endpoint 后 provider 初始化成功(资源 schema 合并等)。
func TestSetupWithEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	shutdown, err := Setup(context.Background(), "test-service", "test")
	if err != nil {
		t.Fatalf("配置 endpoint 的 Setup 不应报错: %v", err)
	}
	// exporter 是懒连接,shutdown 即使连不上 collector 也应尽力返回。
	_ = shutdown(context.Background())
}
