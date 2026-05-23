#!/usr/bin/env bash
# Manual smoke: content-hash deduplication at ingest (PCMI-011).
#
# Usage:
#   make infra-up && make smoke-dedup
#   ./scripts/smoke_dedup.sh
#
# Env:
#   PCMI_BASE_URL   default http://localhost:8000
#   PCMI_API_KEY    default testkey123
#   SKIP_READY=1    skip GET /v1/ready

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BASE="${PCMI_BASE_URL:-http://localhost:8000}"
BASE="${BASE%/}"
KEY="${PCMI_API_KEY:-testkey123}"

GREEN="\033[1;32m"
YELLOW="\033[1;33m"
RED="\033[1;31m"
CYAN="\033[1;36m"
NC="\033[0m"

log()  { echo -e "${CYAN}==>${NC} $*"; }
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*" >&2; }
fail() { echo -e "${RED}[FAIL]${NC} $*" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "comando mancante: $1"
}

HDR=( -H "Content-Type: application/json" -H "X-API-Key: ${KEY}" )

wait_ready() {
  if [[ "${SKIP_READY:-}" == "1" ]]; then
    warn "SKIP_READY=1 — salto /v1/ready"
    return 0
  fi
  log "GET ${BASE}/v1/ready"
  if ! curl -sf "${BASE}/v1/ready" | jq -e '.status == "ready" and .database_ok == true and .redis_ok == true' >/dev/null; then
    fail "API non pronta su ${BASE} (prova: make infra-up)"
  fi
  ok "API ready"
}

main() {
  need_cmd curl
  need_cmd jq

  SUFFIX="$(date +%s)"
  PATH_SKIP="root.smoke.dedup.skip.${SUFFIX}"
  PATH_LINK_CANON="root.smoke.dedup.canon.${SUFFIX}"
  PATH_LINK_ALIAS="root.smoke.dedup.alias.${SUFFIX}"
  CONTENT="dedup-smoke-${SUFFIX}"

  echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
  echo -e "${CYAN}  PCMI smoke: deduplication (PCMI-011)${NC}"
  echo -e "${CYAN}  BASE=${BASE}  suffix=${SUFFIX}${NC}"
  echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"

  wait_ready

  log "POST /v1/memories (first store, skip mode)"
  first="$(curl -sf -X POST "${BASE}/v1/memories" \
    "${HDR[@]}" -H "X-Dedup-Mode: skip" \
    -d "$(jq -n --arg path "$PATH_SKIP" --arg content "$CONTENT" '{path: $path, content: $content, embedding_model: "unspecified"}')")"
  FIRST_ID="$(echo "$first" | jq -r '.id // empty')"
  if [[ -z "$FIRST_ID" ]]; then
    echo "$first" | jq . >&2
    fail "first store: id mancante"
  fi
  ok "first store id=${FIRST_ID}"

  log "POST /v1/memories (same path/content — expect skip)"
  second="$(curl -sf -X POST "${BASE}/v1/memories" \
    "${HDR[@]}" -H "X-Dedup-Mode: skip" \
    -d "$(jq -n --arg path "$PATH_SKIP" --arg content "$CONTENT" '{path: $path, content: $content, embedding_model: "unspecified"}')")"
  SECOND_ACTION="$(echo "$second" | jq -r '.dedup_action // empty')"
  SECOND_ID="$(echo "$second" | jq -r '.id // empty')"
  if [[ "$SECOND_ACTION" != "skipped" || "$SECOND_ID" != "$FIRST_ID" ]]; then
    echo "$second" | jq . >&2
    fail "expected dedup skip same id, got action=${SECOND_ACTION} id=${SECOND_ID}"
  fi
  ok "dedup skip: action=${SECOND_ACTION} id=${SECOND_ID}"

  log "POST /v1/memories (canonical for link mode)"
  curl -sf -X POST "${BASE}/v1/memories" \
    "${HDR[@]}" \
    -d "$(jq -n --arg path "$PATH_LINK_CANON" --arg content "$CONTENT" '{path: $path, content: $content, embedding_model: "unspecified"}')" >/dev/null

  log "POST /v1/memories (alias path, link mode)"
  linked="$(curl -sf -X POST "${BASE}/v1/memories" \
    "${HDR[@]}" -H "X-Dedup-Mode: link" \
    -d "$(jq -n --arg path "$PATH_LINK_ALIAS" --arg content "$CONTENT" '{path: $path, content: $content, embedding_model: "unspecified"}')")"
  LINK_ACTION="$(echo "$linked" | jq -r '.dedup_action // empty')"
  LINK_FROM="$(echo "$linked" | jq -r '.linked_from // empty')"
  if [[ "$LINK_ACTION" != "linked" || "$LINK_FROM" != "$PATH_LINK_ALIAS" ]]; then
    echo "$linked" | jq . >&2
    fail "expected link dedup, got action=${LINK_ACTION} linked_from=${LINK_FROM}"
  fi
  ok "dedup link: action=${LINK_ACTION} linked_from=${LINK_FROM}"

  ok "smoke dedup completato"
}

main "$@"
