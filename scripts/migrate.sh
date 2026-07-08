#!/usr/bin/env bash
set -euo pipefail

ACTION="${1:-up}"
COMPOSE="${COMPOSE:-docker compose -f deploy/docker-compose.yml}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-stableagent}"
POSTGRES_DB="${POSTGRES_DB:-stableagent}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-db/migrations}"

shopt -s nullglob

psql_exec() {
  ${COMPOSE} exec -T "${POSTGRES_SERVICE}" psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" "$@"
}

ensure_table() {
  psql_exec -c "create table if not exists schema_migrations (version varchar(32) primary key, filename text not null, applied_at timestamptz not null default now());" >/dev/null
}

version_from_file() {
  basename "$1" | cut -d_ -f1
}

escaped_sql_value() {
  printf "%s" "$1" | sed "s/'/''/g"
}

apply_up() {
  ensure_table
  local files=("${MIGRATIONS_DIR}"/*.up.sql)
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "没有发现 up migration"
    return 0
  fi

  for file in "${files[@]}"; do
    local version filename applied escaped_filename
    version="$(version_from_file "${file}")"
    filename="$(basename "${file}")"
    applied="$(psql_exec -Atc "select 1 from schema_migrations where version = '${version}' limit 1;" || true)"
    if [[ "${applied}" == "1" ]]; then
      echo "跳过已应用 migration ${filename}"
      continue
    fi

    echo "应用 migration ${filename}"
    psql_exec -f - < "${file}"
    escaped_filename="$(escaped_sql_value "${filename}")"
    psql_exec -c "insert into schema_migrations(version, filename) values ('${version}', '${escaped_filename}');" >/dev/null
  done
}

apply_down() {
  ensure_table
  local latest files file filename
  latest="$(psql_exec -Atc "select version from schema_migrations order by version desc limit 1;" || true)"
  if [[ -z "${latest}" ]]; then
    echo "没有可回滚的 migration"
    return 0
  fi

  files=("${MIGRATIONS_DIR}/${latest}"_*.down.sql)
  if [[ ${#files[@]} -eq 0 ]]; then
    echo "找不到版本 ${latest} 的 down migration"
    exit 1
  fi

  file="${files[0]}"
  filename="$(basename "${file}")"
  echo "回滚 migration ${filename}"
  psql_exec -f - < "${file}"
  psql_exec -c "delete from schema_migrations where version = '${latest}';" >/dev/null
}

show_status() {
  ensure_table
  psql_exec -c "select version, filename, applied_at from schema_migrations order by version;"
}

case "${ACTION}" in
  up)
    apply_up
    ;;
  down)
    apply_down
    ;;
  status)
    show_status
    ;;
  *)
    echo "用法: $0 {up|down|status}"
    exit 1
    ;;
esac
