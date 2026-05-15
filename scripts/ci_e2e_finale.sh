#!/usr/bin/env bash
# CI E2E: distillation finale (from test_finale_v1.1.sh, compose stack already up).
set -euo pipefail

API_URL="${API_URL:-http://localhost:8000}"
API_KEY="${API_KEY:-testkey123}"
SUFFIX="${SUFFIX:-$(date +%s)}"
DISTILL_WAIT_SECS="${DISTILL_WAIT_SECS:-120}"

hdr=(-H "Content-Type: application/json" -H "X-API-Key: ${API_KEY}")

log() { echo "[ci-e2e-finale] $*"; }
fail() { echo "[ci-e2e-finale] FAIL: $*" >&2; exit 1; }

curl -sf "${API_URL}/v1/health" >/dev/null || fail "API not healthy"

MEM_PREFIX="root.test.finale.${SUFFIX}"
for i in 1 2 3; do
  curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
    -d "{\"path\":\"${MEM_PREFIX}.${i}\",\"content\":\"Scalping strategy works with high volume and RSI under 30 on 5m timeframe\",\"metadata\":{\"test\":\"finale\",\"topic\":\"scalping\"},\"embedding_model\":\"text-embedding-3-small\"}" \
    | jq -e '.id' >/dev/null
done

log "waiting up to ${DISTILL_WAIT_SECS}s for distillation at root.test.distilled..."
i=0
while [ "$i" -lt "$DISTILL_WAIT_SECS" ]; do
  distilled=$(curl -sf "${API_URL}/v1/distilled?path_prefix=root.test&limit=20" -H "X-API-Key: ${API_KEY}" \
    | jq '[.entries[] | select(.path == "root.test.distilled")] | length')
  if [ "${distilled:-0}" -ge 1 ]; then
    log "distilled entries=${distilled} after ${i}s"
    break
  fi
  sleep 10
  i=$((i + 10))
done

distilled=$(curl -sf "${API_URL}/v1/distilled?path_prefix=root.test&limit=20" -H "X-API-Key: ${API_KEY}" \
  | jq '[.entries[] | select(.path == "root.test.distilled")] | length')
[ "${distilled:-0}" -ge 1 ] || fail "no distilled knowledge at root.test.distilled after ${DISTILL_WAIT_SECS}s"

retrieve_n=$(curl -sf -X POST "${API_URL}/v1/retrieve" "${hdr[@]}" \
  -d "{\"path_prefix\":\"${MEM_PREFIX}\",\"limit\":10}" | jq '.entries | length')
[ "${retrieve_n:-0}" -ge 1 ] || fail "retrieve empty for ${MEM_PREFIX}"

log "finale E2E passed"
