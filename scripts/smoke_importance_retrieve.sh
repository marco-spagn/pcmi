#!/usr/bin/env bash
# Manual smoke: importance scoring on POST /v1/retrieve and PATCH /v1/memories/{path}/importance.
#
# Usage:
#   make infra-up && make smoke-importance
#   # or:
#   ./scripts/smoke_importance_retrieve.sh
#
# Env:
#   PCMI_BASE_URL   default http://localhost:8000
#   PCMI_API_KEY    default testkey123 (seed in migrations)
#   SKIP_READY=1    skip GET /v1/ready (API already verified)

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
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
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

store_memory() {
  local path="$1" importance="$2" content="$3"
  log "POST /v1/memories path=${path} importance=${importance}"
  curl -sf -X POST "${BASE}/v1/memories" \
    "${HDR[@]}" \
    -d "$(jq -n \
      --arg p "$path" \
      --arg c "$content" \
      --argjson imp "$importance" \
      '{path: $p, content: $c, metadata: {smoke: "importance"}, importance: $imp, embedding_model: "unspecified"}')" \
    | jq -e '.id and (.version | type) == "number"' >/dev/null
  ok "memoria salvata: ${path}"
}

retrieve_ranked() {
  local label="$1"
  log "${label}: POST /v1/retrieve (decay_enabled=false)"
  local resp
  resp="$(curl -sf -X POST "${BASE}/v1/retrieve" \
    "${HDR[@]}" \
    -d "$(jq -n \
      --arg prefix "$PATH_PREFIX" \
      --arg q "$QUERY" \
      '{path_prefix: $prefix, query: $q, limit: 10, decay_enabled: false}')")"
  echo "$resp" | jq -r '.entries[] | "\(.path)\timportance=\(.importance // "n/a")\tscore=\(.relevance_score // "n/a")"'
  FIRST_PATH="$(echo "$resp" | jq -r '.entries[0].path // empty')"
  SECOND_PATH="$(echo "$resp" | jq -r '.entries[1].path // empty')"
  TOTAL="$(echo "$resp" | jq -r '.total // 0')"
  if [[ "$TOTAL" -lt 2 ]]; then
    echo "$resp" | jq . >&2
    fail "retrieve atteso >= 2 risultati, got ${TOTAL}"
  fi
  echo "$resp" | jq '{total, first: .entries[0] | {path, importance, relevance_score}, second: .entries[1] | {path, importance, relevance_score}}'
}

patch_importance() {
  local path="$1" importance="$2"
  local url_path
  url_path="$(python3 -c 'import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=""))' "$path")"
  log "PATCH /v1/memories/${path}/importance → ${importance}"
  curl -sf -X PATCH "${BASE}/v1/memories/${url_path}/importance" \
    "${HDR[@]}" \
    -d "$(jq -n --argjson imp "$importance" '{importance: $imp}')" \
    | jq -e --arg p "$path" --argjson imp "$importance" \
      '.path == $p and .importance == $imp and .status == "updated"' >/dev/null
  ok "importance aggiornata su ${path}"
}

assert_first() {
  local expected="$1" phase="$2"
  if [[ "$FIRST_PATH" != "$expected" ]]; then
    fail "${phase}: atteso primo=${expected}, got primo=${FIRST_PATH:-<vuoto>} secondo=${SECOND_PATH:-<vuoto>}"
  fi
  ok "${phase}: primo in classifica = ${expected}"
}

main() {
  need_cmd curl
  need_cmd jq
  need_cmd python3

  SUFFIX="$(date +%s)"
  PATH_PREFIX="test.importance.${SUFFIX}"
  PATH_A="${PATH_PREFIX}.a"
  PATH_B="${PATH_PREFIX}.b"
  QUERY="zorblax flibbert importance smoke retrieve"
  SHARED_CONTENT="zorblax flibbert importance smoke retrieve scoring tie-break"

  echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
  echo -e "${CYAN}  PCMI smoke: importance + retrieve (PCMI-009)${NC}"
  echo -e "${CYAN}  BASE=${BASE}  prefix=${PATH_PREFIX}${NC}"
  echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"

  wait_ready

  store_memory "$PATH_A" 0.9 "$SHARED_CONTENT"
  store_memory "$PATH_B" 0.2 "$SHARED_CONTENT"

  retrieve_ranked "Fase 1 (A=0.9, B=0.2)"
  assert_first "$PATH_A" "Fase 1"

  patch_importance "$PATH_B" 0.95

  retrieve_ranked "Fase 2 (B patch 0.95)"
  assert_first "$PATH_B" "Fase 2"

  echo ""
  ok "smoke importance/retrieve completato"
}

main "$@"
