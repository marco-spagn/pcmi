#!/usr/bin/env bash
# CI E2E: embedding pipeline (from test_embedding.sh, compose stack already up).
set -euo pipefail

API_URL="${API_URL:-http://localhost:8000}"
API_KEY="${API_KEY:-testkey123}"
SUFFIX="${SUFFIX:-$(date +%s)}"
EMBED_WAIT_SECS="${EMBED_WAIT_SECS:-90}"

hdr=(-H "Content-Type: application/json" -H "X-API-Key: ${API_KEY}")

log() { echo "[ci-e2e-embedding] $*"; }
fail() { echo "[ci-e2e-embedding] FAIL: $*" >&2; exit 1; }

curl -sf "${API_URL}/v1/health" >/dev/null || fail "API not healthy"

PATH_V="root.test.embedding.deep.${SUFFIX}"
log "storing memory at ${PATH_V}"
curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"${PATH_V}\",\"content\":\"Scalping works best with high volume and RSI under 30 on 5m charts\",\"metadata\":{\"test\":\"embedding-deep\"},\"embedding_model\":\"text-embedding-3-small\",\"tags\":[\"scalping\",\"trading\"]}" \
  | jq -e '.id' >/dev/null

log "waiting up to ${EMBED_WAIT_SECS}s for embeddings..."
i=0
while [ "$i" -lt "$EMBED_WAIT_SECS" ]; do
  has=$(docker compose exec -T postgres psql -U pcmi -d pcmi -tA -c \
    "SELECT embedding IS NOT NULL FROM memory_entries WHERE path='${PATH_V}'::ltree AND valid_to IS NULL LIMIT 1" \
    | tr -d '[:space:]')
  if [ "$has" = "t" ]; then
    log "embedding present after ${i}s"
    break
  fi
  sleep 5
  i=$((i + 5))
done
[ "$has" = "t" ] || fail "embedding not generated in time"

n=$(curl -sf -X POST "${API_URL}/v1/retrieve" "${hdr[@]}" \
  -d "{\"path_prefix\":\"root.test.embedding\",\"limit\":8}" | jq '.entries | length')
[ "${n:-0}" -ge 1 ] || fail "retrieve returned no entries"

log "embedding E2E passed"
