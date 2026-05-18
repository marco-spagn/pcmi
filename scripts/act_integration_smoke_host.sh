#!/usr/bin/env bash
# Host-side integration smoke — parity with the GitHub Actions "integration-smoke" job,
# but runs outside nektos/act (act's Docker network layout often breaks localhost bridges
# between the runner container and service containers).
#
# Prerequisites: Docker daemon, Go toolchain, same tools as CI (curl, jq, python3, psql
# optional if you only use compose-init migrations).
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
#   PCMI_EXPECT_VERSION / EXPECT_API_VERSION — same as CI (default v1.33.0).

set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
FRESH="${ACT_SMOKE_FRESH_DB:-1}"

export PCMI_EXPECT_VERSION="${PCMI_EXPECT_VERSION:-v1.33.0}"
export EXPECT_API_VERSION="${EXPECT_API_VERSION:-v1.33.0}"
export PGHOST="${PGHOST:-127.0.0.1}"
export API="${API:-http://127.0.0.1:8000}"

if [ "$FRESH" = "1" ]; then
  echo "[act-integration-smoke] resetting compose volumes (ACT_SMOKE_FRESH_DB=1)"
  $DOCKER_COMPOSE down -v --remove-orphans || true
fi

echo "[act-integration-smoke] starting postgres + redis"
$DOCKER_COMPOSE up -d postgres redis

echo "[act-integration-smoke] waiting for Postgres on ${PGHOST}:5432 …"
for i in $(seq 1 60); do
  if PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -c 'SELECT 1' >/dev/null 2>&1; then
    break
  fi
  sleep 1
  if [ "$i" -eq 60 ]; then
    echo "Postgres did not become reachable — docker compose logs postgres: "
    $DOCKER_COMPOSE logs --tail=50 postgres || true
    exit 1
  fi
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
go build -o bin/pcmi-api ./cmd/api
go build -o bin/pcmi-worker ./cmd/worker

rm -f api.log worker.log
./bin/pcmi-api > api.log 2>&1 &
api_pid=$!
./bin/pcmi-worker > worker.log 2>&1 &
worker_pid=$!

cleanup() {
  kill "$api_pid" "$worker_pid" 2>/dev/null || true
  wait "$api_pid" 2>/dev/null || true
  wait "$worker_pid" 2>/dev/null || true
}
trap cleanup EXIT

echo "[act-integration-smoke] waiting for HTTP /v1/health …"
ok=0
for i in $(seq 1 45); do
  if curl -sf "http://127.0.0.1:${API_PORT}/v1/health" >/dev/null; then
    ok=1
    echo "API healthy"
    break
  fi
  sleep 2
done
if [ "$ok" != "1" ]; then
  echo "[act-integration-smoke] API did not become healthy"
  cat api.log || true
  cat worker.log || true
  exit 1
fi

chmod +x scripts/ci_integration_smoke.sh scripts/ci_sdk_smoke.sh
./scripts/ci_integration_smoke.sh

GRPC_HOST="${GRPC_HOST:-127.0.0.1:${GRPC_PORT}}" GRPC_TEST_API_KEY=testkey123 \
  DATABASE_URL="$DATABASE_URL" \
  go test -tags=integration -count=1 ./internal/grpc/...

PCMI_BASE_URL="${PCMI_BASE_URL:-http://127.0.0.1:${API_PORT}}" PCMI_API_KEY=testkey123 \
  ./scripts/ci_sdk_smoke.sh

echo "[act-integration-smoke] all checks passed"
