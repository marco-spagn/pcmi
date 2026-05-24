#!/usr/bin/env bash
# Host-side integration smoke — parity with the GitHub Actions "integration-smoke" job,
# but runs outside nektos/act (act's Docker network layout often breaks localhost bridges
# between the runner container and service containers).
#
# Prerequisites: Docker daemon, Go toolchain, curl, jq, python3, and **psql** on
# the host (the smoke script runs several direct psql checks against localhost).
#   brew install libpq && brew link --force libpq   # macOS
#
# Usage:
#   bash scripts/act_integration_smoke_host.sh
#
# Fresh database (recommended, matches CI):
#   ACT_SMOKE_FRESH_DB=1 bash scripts/act_integration_smoke_host.sh   # default
#
# Env:
#   ACT_SMOKE_FRESH_DB — if 1 (default), runs `docker compose down -v` first (destructive).
#   DOCKER_COMPOSE — override for the compose binary (default: "docker compose").
#   PCMI_EXPECT_VERSION / EXPECT_API_VERSION — from scripts/ci/resolve_version.sh if unset.
#   Do not use `docker compose up --wait` for first-time Postgres: Compose marks the
#   service healthy while initdb's bootstrap instance still runs migrations; the
#   published :5432 port is only reliable after the final restart. We wait using
#   `docker compose exec … psql` (no host psql required for this step).

set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
FRESH="${ACT_SMOKE_FRESH_DB:-1}"

# shellcheck source=compose_postgres_wait.inc.sh
source "$ROOT/scripts/compose_postgres_wait.inc.sh"

# shellcheck source=ci/resolve_version.sh
source "$ROOT/scripts/ci/resolve_version.sh"
export PGHOST="${PGHOST:-127.0.0.1}"
export API="${API:-http://127.0.0.1:8000}"

FREE_DEV_PORTS_LABEL="[act-integration-smoke]" bash "$ROOT/scripts/free_dev_ports.sh"

if [ "$FRESH" = "1" ]; then
  echo "[act-integration-smoke] resetting compose volumes (ACT_SMOKE_FRESH_DB=1)"
  $DOCKER_COMPOSE down -v --remove-orphans || true
fi

echo "[act-integration-smoke] starting postgres + redis"
# Never use `up --wait` here: the Postgres image healthcheck can pass during the
# temporary server used for docker-entrypoint-initdb.d, before the final server
# binds the published port — host/API probes then race and fail.
$DOCKER_COMPOSE up -d postgres redis

COMPOSE_POSTGRES_WAIT_LABEL="[act-integration-smoke]"
echo "[act-integration-smoke] waiting for Postgres (up to ~360s)…"
if ! compose_wait_postgres_ready 360; then
  echo "Postgres did not become reachable — docker compose logs postgres: "
  $DOCKER_COMPOSE logs --tail=120 postgres || true
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "ERROR: psql is required on the host for scripts/ci_integration_smoke.sh (e.g. macOS: brew install libpq && brew link --force libpq)"
  exit 1
fi

echo "[act-integration-smoke] confirming ${PGHOST}:5432 from host …"
for i in $(seq 1 60); do
  if PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -c 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "Postgres is stable inside Docker but ${PGHOST}:5432 is not reachable from the host (check Docker Desktop port publishing)."
    $DOCKER_COMPOSE logs --tail=40 postgres || true
    exit 1
  fi
  sleep 1
done

# Schema: docker-compose applies migrations/*.sql via docker-entrypoint-initdb.d on
# first boot (same files as CI). Do not re-apply here — duplicate DDL would fail.

mkdir -p bin
export DATABASE_URL="postgres://pcmi:pcmi@${PGHOST}:5432/pcmi?sslmode=disable"
export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
export API_PORT="${API_PORT:-8000}"
export GRPC_PORT="${GRPC_PORT:-50051}"
export RATE_LIMIT_DISABLED="${RATE_LIMIT_DISABLED:-true}"
export PCMI_ENCRYPTION_KEY="${PCMI_ENCRYPTION_KEY:-01234567890123456789012345678901}"
export PRUNE_INTERVAL_SECS="${PRUNE_INTERVAL_SECS:-3600}"
export EXPIRY_INTERVAL_SECS="${EXPIRY_INTERVAL_SECS:-2}"
export WEBHOOK_MAX_ATTEMPTS="${WEBHOOK_MAX_ATTEMPTS:-2}"

echo "[act-integration-smoke] building API + worker"
rm -f api.log worker.log api.pid worker.pid
chmod +x scripts/ci/start_api_worker.sh scripts/ci/wait_api_health.sh scripts/ci/stop_api_worker.sh
scripts/ci/start_api_worker.sh

cleanup() {
  scripts/ci/stop_api_worker.sh
}
trap cleanup EXIT

echo "[act-integration-smoke] waiting for HTTP /v1/health …"
API_URL="http://127.0.0.1:${API_PORT}/v1/health" scripts/ci/wait_api_health.sh || {
  cat api.log || true
  cat worker.log || true
  exit 1
}

chmod +x scripts/ci_integration_smoke.sh scripts/ci_sdk_smoke.sh
./scripts/ci_integration_smoke.sh

GRPC_HOST="${GRPC_HOST:-127.0.0.1:${GRPC_PORT}}" GRPC_TEST_API_KEY=testkey123 \
  DATABASE_URL="$DATABASE_URL" \
  go test -tags=integration -count=1 ./internal/grpc/...

PCMI_BASE_URL="${PCMI_BASE_URL:-http://127.0.0.1:${API_PORT}}" PCMI_API_KEY=testkey123 \
  ./scripts/ci_sdk_smoke.sh

echo "[act-integration-smoke] all checks passed"
