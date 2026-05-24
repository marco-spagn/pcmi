#!/usr/bin/env bash
# Build and start API + worker in background; writes api.pid / worker.pid and logs.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
mkdir -p bin

export DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
export REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
export API_PORT="${API_PORT:-8000}"
export GRPC_PORT="${GRPC_PORT:-50051}"
export RATE_LIMIT_DISABLED="${RATE_LIMIT_DISABLED:-true}"
export PCMI_ENCRYPTION_KEY="${PCMI_ENCRYPTION_KEY:-01234567890123456789012345678901}"
export PRUNE_INTERVAL_SECS="${PRUNE_INTERVAL_SECS:-3600}"
export EXPIRY_INTERVAL_SECS="${EXPIRY_INTERVAL_SECS:-2}"
export WEBHOOK_MAX_ATTEMPTS="${WEBHOOK_MAX_ATTEMPTS:-2}"

go build -o bin/pcmi-api ./cmd/api
go build -o bin/pcmi-worker ./cmd/worker

./bin/pcmi-api >api.log 2>&1 &
echo $! >api.pid
./bin/pcmi-worker >worker.log 2>&1 &
echo $! >worker.pid
