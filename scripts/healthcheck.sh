#!/usr/bin/env bash
set -euo pipefail

check() {
  local name="$1"
  local url="$2"
  echo "检查 ${name}: ${url}"
  curl --fail --silent --show-error "${url}" >/dev/null
}

check "api-service" "http://127.0.0.1:8080/healthz"
check "runtime-worker" "http://127.0.0.1:8081/healthz"
check "tool-gateway" "http://127.0.0.1:18082/healthz"
check "llm-gateway" "http://127.0.0.1:8083/healthz"

echo "四个服务 health check 均通过"
