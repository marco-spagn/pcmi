#!/usr/bin/env bash
# Wait for docker-compose Postgres (and optionally an HTTP URL) to become ready.
# Used by Makefile infra-wait / infra-wait-db targets.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
WAIT_HTTP="${1:-}"
HTTP_MAX_SECS="${HTTP_MAX_SECS:-180}"
PG_MAX_SECS="${PG_MAX_SECS:-180}"

wait_postgres() {
	local i=0
	while [ "$i" -lt "$PG_MAX_SECS" ]; do
		if $DOCKER_COMPOSE exec -T postgres psql -U pcmi -d pcmi -c 'SELECT 1' >/dev/null 2>&1; then
			echo "[infra] postgres ready"
			return 0
		fi
		sleep 1
		i=$((i + 1))
	done
	echo "[infra] postgres not ready after ${PG_MAX_SECS}s (is 'make infra-deps-up' or 'make infra-up' running?)" >&2
	return 1
}

wait_http() {
	local url="$1"
	local i=0
	while [ "$i" -lt "$HTTP_MAX_SECS" ]; do
		if curl -sf "$url" >/dev/null 2>&1; then
			echo "[infra] $url ready"
			return 0
		fi
		sleep 2
		i=$((i + 2))
	done
	echo "[infra] $url not ready after ${HTTP_MAX_SECS}s (check: make infra-logs)" >&2
	return 1
}

if [ -z "$($DOCKER_COMPOSE ps -q postgres 2>/dev/null)" ]; then
	echo "[infra] postgres container not found — run 'make infra-deps-up' or 'make infra-up' first" >&2
	exit 1
fi

wait_postgres

if [ -n "$WAIT_HTTP" ]; then
	wait_http "$WAIT_HTTP"
fi
