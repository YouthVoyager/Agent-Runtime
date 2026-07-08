#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-/tmp/stableagent-verify}"
mkdir -p "${LOG_DIR}"

pids=()

cleanup() {
  for pid in "${pids[@]}"; do
    kill "${pid}" >/dev/null 2>&1 || true
  done
  for pid in "${pids[@]}"; do
    wait "${pid}" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

require_binary() {
  local name="$1"
  if [[ ! -x "./bin/${name}" ]]; then
    echo "缺少 ./bin/${name}，请先运行 make build"
    exit 1
  fi
}

start_service() {
  local name="$1"
  local env_key="$2"
  local port="$3"
  local log_file="${LOG_DIR}/${name}.log"

  echo "启动 ${name}，端口 ${port}"
  env "${env_key}=:${port}" "./bin/${name}" >"${log_file}" 2>&1 &
  pids+=("$!")
}

wait_health() {
  local name="$1"
  local port="$2"
  local log_file="${LOG_DIR}/${name}.log"
  local url="http://127.0.0.1:${port}/healthz"

  for _ in {1..50}; do
    if curl --fail --silent --show-error "${url}" >/dev/null 2>&1; then
      echo "${name} health check 通过"
      return 0
    fi
    sleep 0.2
  done

  echo "${name} health check 失败，日志如下："
  cat "${log_file}" || true
  return 1
}

require_binary "api-service"
require_binary "runtime-worker"
require_binary "tool-gateway"
require_binary "llm-gateway"

# tool-gateway 备用端口避开 18082:该端口是 docker-compose 中 tool-gateway 的对外映射,
# Docker 环境运行时会命中容器而非被测二进制,导致冒烟结果失真。
start_service "api-service" "API_SERVICE_HTTP_ADDR" "18080"
start_service "runtime-worker" "RUNTIME_WORKER_HTTP_ADDR" "18081"
start_service "tool-gateway" "TOOL_GATEWAY_HTTP_ADDR" "18084"
start_service "llm-gateway" "LLM_GATEWAY_HTTP_ADDR" "18083"

wait_health "api-service" "18080"
wait_health "runtime-worker" "18081"
wait_health "tool-gateway" "18084"
wait_health "llm-gateway" "18083"

echo "四个服务均可正常启动并响应 health check"
