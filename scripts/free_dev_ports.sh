#!/usr/bin/env bash
# Free host ports 5432 (Postgres) and 6379 (Redis) for pcmi compose and nektos/act.
#
# - Stops this repo's docker compose stack (pcmi postgres/redis/api/worker).
# - Stops any Docker container still publishing those ports (e.g. leftover act-* services).
# Does not touch non-Docker processes on those ports.
#
# Opt out (keep compose / ports as-is): SKIP_ACT_PORT_CLEANUP=1
#
# Usage:
#   bash scripts/free_dev_ports.sh
#   FREE_DEV_PORTS_LABEL="[my-step]" bash scripts/free_dev_ports.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
LABEL="${FREE_DEV_PORTS_LABEL:-[free-dev-ports]}"

if [[ "${SKIP_ACT_PORT_CLEANUP:-}" == "1" ]]; then
  echo "${LABEL} SKIP_ACT_PORT_CLEANUP=1 — leaving compose and published ports as-is"
  exit 0
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "${LABEL} docker not found — skipping cleanup"
  exit 0
fi

if ! docker info >/dev/null 2>&1; then
  echo "${LABEL} Docker daemon not running — skipping cleanup"
  exit 0
fi

echo "${LABEL} docker compose down — freeing :5432 / :6379 for compose and act"
$DOCKER_COMPOSE down --remove-orphans 2>/dev/null || true
docker rm -f pcmi-api pcmi-worker 2>/dev/null || true

stop_published_port() {
  local port="$1"
  local ids
  ids="$(docker ps --filter "publish=${port}" -q 2>/dev/null || true)"
  if [[ -z "${ids}" ]]; then
    return 0
  fi
  echo "${LABEL} stopping containers publishing :${port} ($(echo "${ids}" | wc -w | tr -d ' ') container(s))"
  # shellcheck disable=SC2086
  docker stop ${ids} 2>/dev/null || true
  # shellcheck disable=SC2086
  docker rm -f ${ids} 2>/dev/null || true
}

stop_published_port 5432
stop_published_port 6379

for port in 5432 6379; do
  for _ in $(seq 1 10); do
    if [[ -z "$(docker ps --filter "publish=${port}" -q 2>/dev/null || true)" ]]; then
      break
    fi
    sleep 0.3
  done
done
