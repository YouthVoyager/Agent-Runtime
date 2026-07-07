#!/usr/bin/env bash
set -euo pipefail

check() {
  local name="$1"
  local url="$2"
  echo "检查 ${name}: ${url}"
  curl --fail --silent --show-error "${url}" >/dev/null
}

check "api-service" "http://localhost:8080/healthz"
check "runtime-worker" "http://localhost:8081/healthz"
check "tool-gateway" "http://localhost:8082/healthz"
check "llm-gateway" "http://localhost:8083/healthz"

echo "四个服务 health check 均通过"
