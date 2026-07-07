#!/usr/bin/env bash
set -euo pipefail

SERVICE="${1:-}"
if [[ -z "${SERVICE}" ]]; then
  echo "用法: $0 {api-service|runtime-worker|tool-gateway|llm-gateway}"
  exit 1
fi

go run "./cmd/${SERVICE}"
