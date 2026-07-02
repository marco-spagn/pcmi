#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# One-command cold start for the PCMI × TI Mindmap HUB PoC.
#
# Brings up the AGE-enabled Postgres + Redis (compose "graph" profile), applies
# the two migrations the compose initdb mount list does NOT include (020, 021),
# starts the PCMI API + worker with the OpenAI key from the repo .env (so the
# worker computes LLM embeddings for semantic retrieval), waits for readiness,
# then runs the PoC.
#
# Usage:
#   scripts/run_e2e.sh            # cold start (reuse DB volume if present) + run
#   scripts/run_e2e.sh --fresh    # wipe the DB volume first (true cold start) + run
#   scripts/run_e2e.sh down       # stop API/worker + compose stack
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

POC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$POC_DIR/.." && pwd)"
RUN_DIR="$POC_DIR/.run"
PGURL="postgres://pcmi:pcmi@localhost:5433/pcmi?sslmode=disable"
export PGPASSWORD=pcmi
mkdir -p "$RUN_DIR"

stop_services() {
  for svc in api worker; do
    [ -f "$RUN_DIR/$svc.pid" ] && kill "$(cat "$RUN_DIR/$svc.pid")" 2>/dev/null || true
    rm -f "$RUN_DIR/$svc.pid"
  done
  pkill -f 'go run ./cmd/api'    2>/dev/null || true
  pkill -f 'go run ./cmd/worker' 2>/dev/null || true
  # kill any leftover compiled listener on the API port
  lsof -nP -iTCP:8000 -sTCP:LISTEN 2>/dev/null | awk 'NR>1{print $2}' | xargs -r kill 2>/dev/null || true
}

if [ "${1:-}" = "down" ]; then
  echo "[down] stopping services + compose stack"
  stop_services
  (cd "$REPO_ROOT" && docker compose --profile graph down --remove-orphans)
  exit 0
fi

FRESH=0
[ "${1:-}" = "--fresh" ] && FRESH=1

cd "$REPO_ROOT"

OPENAI_API_KEY="$(grep -E '^OPENAI_API_KEY=' .env 2>/dev/null | head -1 | cut -d= -f2- || true)"
if [ -z "$OPENAI_API_KEY" ]; then
  echo "[warn] no OPENAI_API_KEY in .env — running without LLM embeddings;"
  echo "       semantic actor correlation (Cozy Bear ≡ Midnight Blizzard) will be limited."
fi

echo "[1/6] starting AGE Postgres + Redis (graph profile)…"
if [ "$FRESH" = 1 ]; then
  docker compose --profile graph down -v --remove-orphans >/dev/null 2>&1 || true
fi
docker compose --profile graph up -d postgres-age redis >/dev/null

echo "[2/6] waiting for Postgres health…"
h=none
for _ in $(seq 1 40); do
  h="$(docker inspect --format '{{.State.Health.Status}}' pcmi-postgres-age 2>/dev/null || echo none)"
  if [ "$h" = "healthy" ]; then break; fi
  sleep 3
done
[ "$h" = "healthy" ] || { echo "  ✗ postgres-age not healthy"; exit 1; }

echo "[3/6] applying migrations 020 + 021 (compose initdb only mounts 001–019)…"
# The postgres entrypoint runs its initdb scripts against a temporary server, so
# the healthcheck can report healthy and then the server restarts — wait for a
# stable TCP connection first, then apply with a few retries. (No-op once PR #169
# lands and compose mounts 020/021 itself; these migrations are idempotent.)
for _ in $(seq 1 30); do
  if psql "$PGURL" -tAc "SELECT 1" >/dev/null 2>&1; then break; fi
  sleep 2
done
for mig in 020_memory_open_version_unique 021_link_type_check; do
  applied=0
  for _ in $(seq 1 5); do
    if psql "$PGURL" -v ON_ERROR_STOP=1 -f "migrations/${mig}.sql" >/dev/null 2>&1; then applied=1; break; fi
    sleep 2
  done
  [ "$applied" = 1 ] || { echo "  ✗ failed to apply migrations/${mig}.sql"; exit 1; }
done

emb_state=off; [ -n "$OPENAI_API_KEY" ] && emb_state=on
echo "[4/6] starting PCMI API + worker (embeddings ${emb_state})…"
stop_services
DATABASE_URL="$PGURL" REDIS_ADDR="localhost:6379" EVENT_BACKEND="streams" \
  OPENAI_API_KEY="$OPENAI_API_KEY" EMBEDDING_MODEL="text-embedding-3-small" \
  RATE_LIMIT_DISABLED="true" API_PORT="8000" GRPC_PORT="50051" \
  nohup go run ./cmd/api > "$RUN_DIR/api.log" 2>&1 &
echo $! > "$RUN_DIR/api.pid"
DATABASE_URL="$PGURL" REDIS_ADDR="localhost:6379" EVENT_BACKEND="streams" \
  OPENAI_API_KEY="$OPENAI_API_KEY" EMBEDDING_MODEL="text-embedding-3-small" \
  DISTILLATION_POLICY_DISABLED="true" \
  nohup go run ./cmd/worker > "$RUN_DIR/worker.log" 2>&1 &
echo $! > "$RUN_DIR/worker.pid"

echo "[5/6] waiting for API readiness + graph health…"
for _ in $(seq 1 60); do
  if curl -sf http://localhost:8000/v1/ready >/dev/null 2>&1; then break; fi
  sleep 2
done
curl -s http://localhost:8000/v1/ready; echo
gh="$(curl -s http://localhost:8000/v1/graph/health || true)"
echo "graph: $gh"
echo "$gh" | grep -q '"available":true' || { echo "  ✗ Cognitive Graph not available"; exit 1; }

echo "[6/6] running the PoC (TI_HUB_MODE=${TI_HUB_MODE:-demo})…"
cd "$POC_DIR"
python3 -m pip install -q --user "httpx>=0.27" >/dev/null 2>&1 || true
PCMI_BASE_URL="http://localhost:8000" PCMI_API_KEY="testkey123" \
  TI_HUB_MODE="${TI_HUB_MODE:-demo}" TIHUB_STIX_DIR="${TIHUB_STIX_DIR:-}" \
  TIHUB_REPORTS_DIR="${TIHUB_REPORTS_DIR:-}" TIHUB_API_KEY="${TIHUB_API_KEY:-}" \
  TIHUB_MCP_URL="${TIHUB_MCP_URL:-}" \
  python3 run_poc.py
rc=$?

echo
echo "services still running (logs in $RUN_DIR). Tear down with: scripts/run_e2e.sh down"
exit $rc
