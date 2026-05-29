#!/usr/bin/env bash
# =============================================================================
# Scenario 05 — Distillation idempotency (dedup on source_entry_ids)
# =============================================================================
# Verifies that the worker's `hasDuplicateDistillation` check rejects
# repeated attempts to distill the same 10 records.
#
# Flow:
#   1) Runs scenario 03 (100→10 distilled).
#   2) Does NOT tear down.
#   3) Republishes the same 10 refine events.
#   4) Verifies that distilled_count does NOT grow (idempotency).
#
# Use case: validate the "exactly-once distillation per shard" guarantee.
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

GREEN="\033[1;32m"; YELLOW="\033[1;33m"; RED="\033[1;31m"; NC="\033[0m"
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[ERR]${NC} $*" >&2; }

# Phase 1: standard smoke (does NOT tear down)
echo "[scenario-05] phase 1/3 — first round of distillation (100 → 10)"
"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --no-teardown \
  --num 100 \
  --distill-batch-size 10

# Phase 2: read current stats
TENANT_ID="${TENANT_ID:-a1b2c3d4-e5f6-7890-abcd-ef1234567890}"
PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
KEY_NAME="soc-bulk-loader"
ADMIN_API_KEY="${PCMI_ADMIN_API_KEY:-testkey123}"
KEY_ID="$(curl -fsS -H "X-API-Key: ${ADMIN_API_KEY}" \
  "${PCMI_BASE_URL}/v1/admin/api-keys?tenant_id=${TENANT_ID}&limit=200" \
  | jq -r --arg n "$KEY_NAME" '.api_keys[] | select(.name==$n) | .id' | head -1)"
KEY_RAW="$(curl -fsS -X POST -H "X-API-Key: ${ADMIN_API_KEY}" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg n "$KEY_NAME" '{name:$n}')" \
  "${PCMI_BASE_URL}/v1/admin/api-keys/${KEY_ID}/rotate" | jq -r '.api_key')"

PRE="$(curl -fsS -H "X-API-Key: ${KEY_RAW}" "${PCMI_BASE_URL}/v1/stats" | jq -r '.distilled_count')"
echo "[scenario-05] phase 2/3 — current distilled_count = ${PRE}"

# Phase 3: republish the same 10 refine events
# The worker's dedup check (hasDuplicateDistillation) runs AFTER the
# LLM call: so the worker still makes the call, then realizes it has
# the same source_entry_ids and skips the INSERT.
# Expected result: distilled_count unchanged.
EVENT_BACKEND="${EVENT_BACKEND:-streams}"
publish_memory_event() {
  local payload="$1"
  if [[ "${EVENT_BACKEND}" == "pubsub" ]]; then
    ( cd "$ROOT" && docker compose exec -T redis redis-cli PUBLISH memory_events "$payload" >/dev/null )
    return 0
  fi
  local etype data
  etype=$(echo "$payload" | jq -r '.Type')
  data=$(echo "$payload" | jq -c .)
  printf '%s' "$data" | ( cd "$ROOT" && docker compose exec -T -i redis redis-cli -x XADD pcmi:events '*' type "$etype" data >/dev/null )
}

echo "[scenario-05] phase 3/3 — reapply 10 identical refine events (EVENT_BACKEND=${EVENT_BACKEND})"
for ((i=0; i<10; i++)); do
  PREFIX="$(printf "root.security.incidents.soc.shard_%03d" "$i")"
  PAYLOAD="$(jq -nc --arg t "$TENANT_ID" --arg p "$PREFIX" \
    '{Type:"memory.refine.requested", Payload:{tenant_id:$t, path_prefix:$p, reason:"dedup_retry"}}')"
  publish_memory_event "$PAYLOAD"
  sleep 1.2
done
echo "[scenario-05] waiting 30s for the worker to finish 10 LLM calls + dedup check..."
sleep 30

POST="$(curl -fsS -H "X-API-Key: ${KEY_RAW}" "${PCMI_BASE_URL}/v1/stats" | jq -r '.distilled_count')"
echo "[scenario-05] after retry: distilled_count = ${POST}"

if [[ "$PRE" == "$POST" ]]; then
  ok "Idempotency confirmed: distilled_count unchanged (${PRE} = ${POST})."
  EXIT=0
else
  err "Dedup violated: distilled_count changed (${PRE} → ${POST})."
  EXIT=1
fi

echo "[scenario-05] final teardown"
( cd "$ROOT" && docker compose down -v --remove-orphans >/dev/null 2>&1 ) || true
exit $EXIT
