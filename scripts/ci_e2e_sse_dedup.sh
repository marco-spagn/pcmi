#!/usr/bin/env bash
# CI E2E: SSE /v1/events (memory.stored, knowledge.distilled), distillation dedup,
# and heavier integration checks. Requires docker compose stack + OPENAI_API_KEY.
set -euo pipefail

API_URL="${API_URL:-http://localhost:8000}"
API_KEY="${API_KEY:-testkey123}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000000}"
SUFFIX="${SUFFIX:-$(date +%s)}"
DISTILL_WAIT_SECS="${DISTILL_WAIT_SECS:-120}"
EMBED_WAIT_SECS="${EMBED_WAIT_SECS:-90}"

hdr=(-H "Content-Type: application/json" -H "X-API-Key: ${API_KEY}")

log() { echo "[ci-e2e] $*"; }
fail() { echo "[ci-e2e] FAIL: $*" >&2; exit 1; }

api_health() {
  curl -sf "${API_URL}/v1/health" >/dev/null || fail "API not healthy at ${API_URL}"
}

store_memory() {
  local path=$1 content=$2
  curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
    -d "{\"path\":\"${path}\",\"content\":\"${content}\",\"metadata\":{\"ci\":\"e2e\",\"suffix\":\"${SUFFIX}\"},\"embedding_model\":\"text-embedding-3-small\",\"tags\":[\"ci-e2e\"]}" \
    | jq -e '.id'
}

distilled_path_for_prefix() {
  echo "${1}.distilled"
}

distilled_count_for_prefix() {
  local prefix=$1
  local dpath
  dpath=$(distilled_path_for_prefix "$prefix")
  curl -sf "${API_URL}/v1/distilled?path_prefix=${prefix}&limit=100" -H "X-API-Key: ${API_KEY}" \
    | jq --arg p "$dpath" '[.entries[] | select(.path == $p)] | length'
}

wait_for_distilled_at_least() {
  local prefix=$1 min=$2
  local i=0
  while [ "$i" -lt "$DISTILL_WAIT_SECS" ]; do
    local n
    n=$(distilled_count_for_prefix "$prefix" || echo 0)
    if [ "${n:-0}" -ge "$min" ]; then
      log "distilled count for $(distilled_path_for_prefix "$prefix") >= ${min} (${n}) after ${i}s"
      return 0
    fi
    sleep 5
    i=$((i + 5))
  done
  fail "no distilled knowledge at $(distilled_path_for_prefix "$prefix") after ${DISTILL_WAIT_SECS}s"
}

wait_for_retrieve_nonempty() {
  local prefix=$1
  local i=0
  while [ "$i" -lt "$EMBED_WAIT_SECS" ]; do
    local n
    n=$(curl -sf -X POST "${API_URL}/v1/retrieve" "${hdr[@]}" \
      -d "{\"path_prefix\":\"${prefix}\",\"limit\":20}" | jq '.entries | length')
    if [ "${n:-0}" -ge 1 ]; then
      log "retrieve under ${prefix} returned ${n} entries after ${i}s"
      return 0
    fi
    sleep 5
    i=$((i + 5))
  done
  fail "retrieve empty for ${prefix} after ${EMBED_WAIT_SECS}s (embeddings may not have run)"
}

republish_memory_stored() {
  local path=$1 id=${2:-0}
  local payload
  payload=$(jq -nc --arg t "$TENANT_ID" --arg p "$path" --argjson id "$id" \
    '{Type:"memory.stored",Payload:{id:$id,tenant_id:$t,path:$p}}')
  docker compose exec -T redis redis-cli PUBLISH memory_events "$payload" >/dev/null
}

start_sse_capture() {
  local types=$1 out=$2
  curl -sN --max-time "$((DISTILL_WAIT_SECS + 60))" \
    "${API_URL}/v1/events?types=${types}" \
    -H "X-API-Key: ${API_KEY}" \
    -H "Accept: text/event-stream" >"$out" 2>/dev/null &
  echo $!
}

wait_sse_pattern() {
  local file=$1 pattern=$2 timeout=$3
  local i=0
  while [ "$i" -lt "$timeout" ]; do
    if grep -q "$pattern" "$file" 2>/dev/null; then
      return 0
    fi
    sleep 2
    i=$((i + 2))
  done
  return 1
}

stop_sse() {
  local pid=$1
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

# --- Test 1: SSE memory.stored + knowledge.distilled ---
test_sse_memory_and_distilled() {
  log "TEST 1: SSE memory.stored + knowledge.distilled"
  local event_log sse_pid mem_path
  event_log=$(mktemp)
  local distill_prefix="root.sse"
  mem_path="${distill_prefix}.evt.${SUFFIX}"

  sse_pid=$(start_sse_capture "memory.stored,knowledge.distilled" "$event_log")
  sleep 2

  store_memory "$mem_path" "SSE E2E: distillation trigger for knowledge.distilled event ${SUFFIX}" >/dev/null

  wait_sse_pattern "$event_log" 'memory.stored' 30 \
    || { cat "$event_log" >&2; fail "SSE did not receive memory.stored"; }

  log "memory.stored seen; waiting for distillation + knowledge.distilled (up to ${DISTILL_WAIT_SECS}s)..."
  wait_sse_pattern "$event_log" 'knowledge.distilled' "$DISTILL_WAIT_SECS" \
    || { cat "$event_log" >&2; fail "SSE did not receive knowledge.distilled"; }

  stop_sse "$sse_pid"
  rm -f "$event_log"
  log "TEST 1 OK"
}

# --- Test 2: Distillation dedup (same source_entry_ids / tenant / path) ---
test_distillation_dedup() {
  log "TEST 2: distillation dedup"
  local distill_prefix="root.dedup" dpath p1 p2 id1 id2 before after
  dpath=$(distilled_path_for_prefix "$distill_prefix")
  p1="${distill_prefix}.mem.${SUFFIX}.a"
  p2="${distill_prefix}.mem.${SUFFIX}.b"

  id1=$(store_memory "$p1" "Dedup E2E memory A ${SUFFIX}")
  id2=$(store_memory "$p2" "Dedup E2E memory B ${SUFFIX}")
  log "stored memories id=${id1} id=${id2}"

  wait_for_distilled_at_least "$distill_prefix" 1

  before=$(docker compose exec -T postgres psql -U pcmi -d pcmi -t -A -c \
    "SELECT COUNT(*)::int FROM distilled_knowledge
     WHERE tenant_id = '${TENANT_ID}'::uuid AND path::text = '${dpath}';")
  before=${before//[[:space:]]/}
  log "distilled rows at ${dpath} before retrigger: ${before}"

  # Re-trigger distillation without new memories (same source set → dedup skip)
  republish_memory_stored "$p1" "$id1"
  republish_memory_stored "$p2" "$id2"
  log "republished memory.stored; waiting 45s for duplicate distillation attempt..."
  sleep 45

  after=$(docker compose exec -T postgres psql -U pcmi -d pcmi -t -A -c \
    "SELECT COUNT(*)::int FROM distilled_knowledge
     WHERE tenant_id = '${TENANT_ID}'::uuid AND path::text = '${dpath}';")
  after=${after//[[:space:]]/}

  if [ "$after" -gt "$((before + 1))" ]; then
    docker compose exec -T postgres psql -U pcmi -d pcmi -c \
      "SELECT id, path::text, source_entry_ids, distilled_at FROM distilled_knowledge
       WHERE tenant_id = '${TENANT_ID}'::uuid AND path::text = '${dpath}'
       ORDER BY distilled_at DESC LIMIT 10;" >&2 || true
    fail "distillation dedup broken: count ${before} -> ${after} (expected at most ${before}+1)"
  fi

  # No duplicate rows with identical source_entry_ids
  local dup_groups
  dup_groups=$(docker compose exec -T postgres psql -U pcmi -d pcmi -t -A -c \
    "SELECT COUNT(*) FROM (
       SELECT source_entry_ids FROM distilled_knowledge
       WHERE tenant_id = '${TENANT_ID}'::uuid AND path::text = '${dpath}'
       GROUP BY source_entry_ids HAVING COUNT(*) > 1
     ) d;")
  dup_groups=${dup_groups//[[:space:]]/}
  if [ "${dup_groups:-0}" != "0" ]; then
    fail "found ${dup_groups} duplicate source_entry_ids groups at ${dpath}"
  fi

  log "TEST 2 OK (rows ${before} -> ${after}, no duplicate source sets)"
}

# --- Test 3: SSE type filter + concurrent stores + semantic retrieve ---
test_complex_integration() {
  log "TEST 3: complex integration (filter, concurrent store, semantic retrieve)"
  local filter_log filter_pid prefix i pids=()

  filter_log=$(mktemp)
  filter_pid=$(start_sse_capture "knowledge.distilled" "$filter_log")
  sleep 2

  local distill_prefix="root.complex"
  prefix="${distill_prefix}.mem.${SUFFIX}"
  for i in 1 2 3 4 5; do
  (
    store_memory "${prefix}.${i}" "Concurrent CI memory ${i} about volatility and risk ${SUFFIX}" >/dev/null
  ) &
    pids+=($!)
  done
  for pid in "${pids[@]}"; do wait "$pid" || fail "concurrent store failed"; done
  log "5 concurrent stores finished"

  if wait_sse_pattern "$filter_log" 'memory.stored' 8; then
    stop_sse "$filter_pid"
    rm -f "$filter_log"
    fail "SSE type filter leaked memory.stored when only knowledge.distilled requested"
  fi
  stop_sse "$filter_pid"
  rm -f "$filter_log"

  wait_for_retrieve_nonempty "$prefix"

  local sem_n i=0
  while [ "$i" -lt "$EMBED_WAIT_SECS" ]; do
    sem_n=$(curl -sf -X POST "${API_URL}/v1/retrieve" "${hdr[@]}" \
      -d "{\"path_prefix\":\"${prefix}\",\"query\":\"volatility risk management\",\"limit\":5}" \
      | jq '.entries | length')
    if [ "${sem_n:-0}" -ge 1 ]; then
      log "semantic retrieve returned ${sem_n} entries after ${i}s"
      break
    fi
    sleep 5
    i=$((i + 5))
  done
  if [ "${sem_n:-0}" -lt 1 ]; then
    fail "semantic retrieve returned no entries after ${EMBED_WAIT_SECS}s (embeddings may be pending)"
  fi

  wait_for_distilled_at_least "$distill_prefix" 1

  local insights_ok dpath
  dpath=$(distilled_path_for_prefix "$distill_prefix")
  insights_ok=$(curl -sf "${API_URL}/v1/distilled?path_prefix=${distill_prefix}&limit=20" -H "X-API-Key: ${API_KEY}" \
    | jq --arg p "$dpath" '[.entries[] | select(.path == $p) | (.insights | length > 0)] | any')
  if [ "$insights_ok" != "true" ]; then
    fail "distilled entry missing non-empty insights array"
  fi

  log "TEST 3 OK"
}

main() {
  log "starting E2E (suffix=${SUFFIX})"
  command -v jq >/dev/null || fail "jq required"
  command -v curl >/dev/null || fail "curl required"
  api_health

  test_sse_memory_and_distilled
  test_distillation_dedup
  test_complex_integration

  log "all CI E2E tests passed"
}

main "$@"
