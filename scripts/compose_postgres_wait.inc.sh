# shellcheck shell=bash
# Source from repo root after `cd "$ROOT"` with DOCKER_COMPOSE and FRESH set.
# FRESH=1 means the Postgres data volume was just recreated (wait for initdb log marker).
#
# Provides: compose_wait_postgres_ready MAX_SECONDS → exit 0 when Postgres answers inside the container.

compose_wait_until_log() {
  local needle="$1"
  local max="${2:-360}"
  local i=0
  while [ "$i" -lt "$max" ]; do
    if $DOCKER_COMPOSE logs postgres 2>&1 | grep -Fq "$needle"; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  return 1
}

compose_wait_exec_psql_ok() {
  $DOCKER_COMPOSE exec -T postgres psql -U pcmi -d pcmi -c 'SELECT 1' >/dev/null 2>&1
}

compose_wait_postgres_ready() {
  local max="${1:-360}"
  local i=0
  local label="${COMPOSE_POSTGRES_WAIT_LABEL:-[postgres-wait]}"

  if [ "${FRESH:-0}" = "1" ]; then
    echo "${label} waiting for docker-entrypoint initdb (log marker)…"
    if ! compose_wait_until_log "PostgreSQL init process complete" "$max"; then
      return 1
    fi
    sleep 2
  fi

  echo "${label} waiting for live Postgres inside container…"
  while [ "$i" -lt "$max" ]; do
    if compose_wait_exec_psql_ok; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  return 1
}
