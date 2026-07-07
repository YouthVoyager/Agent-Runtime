package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesScopedEnv(t *testing.T) {
	t.Setenv("APP_CONFIG_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("API_SERVICE_HTTP_ADDR", ":19090")
	t.Setenv("API_SERVICE_LOG_LEVEL", "debug")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")

	cfg, err := Load("api-service", Defaults{HTTPAddr: ":8080"})
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}

	if cfg.HTTPAddr != ":19090" {
		t.Fatalf("HTTPAddr = %q, want :19090", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 3s", cfg.ShutdownTimeout)
	}
}

func TestLoadReadsEnvFileWithoutOverwritingExistingEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "local.env")
	if err := os.WriteFile(envFile, []byte("HTTP_ADDR=:18080\nLOG_LEVEL=warn\n"), 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	t.Setenv("APP_CONFIG_FILE", envFile)
	t.Setenv("LOG_LEVEL", "error")

	cfg, err := Load("tool-gateway", Defaults{})
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}

	if cfg.HTTPAddr != ":18080" {
		t.Fatalf("HTTPAddr = %q, want :18080", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "error" {
		t.Fatalf("LogLevel = %q, want error", cfg.LogLevel)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("APP_CONFIG_FILE", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("READ_TIMEOUT", "invalid")

	if _, err := Load("runtime-worker", Defaults{}); err == nil {
		t.Fatal("Load 未返回非法 duration 错误")
	}
}
