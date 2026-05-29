#!/usr/bin/env bash
# Manual smoke: agent sessions + working memory (PCMI-010).
#
# Usage:
#   make infra-up && make smoke-sessions
#   # or:
#   ./scripts/smoke_sessions.sh
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

main() {
  need_cmd curl
  need_cmd jq

  SUFFIX="$(date +%s)"
  WM_PATH="note.${SUFFIX}"
  WM_CONTENT="session-smoke-working-${SUFFIX}"

  echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"
  echo -e "${CYAN}  PCMI smoke: agent sessions (PCMI-010)${NC}"
  echo -e "${CYAN}  BASE=${BASE}  suffix=${SUFFIX}${NC}"
  echo -e "${CYAN}══════════════════════════════════════════════════════════════${NC}"

  wait_ready

  log "POST /v1/sessions"
  sess_resp="$(curl -sf -X POST "${BASE}/v1/sessions" \
    "${HDR[@]}" \
    -d "$(jq -n --arg suffix "$SUFFIX" '{metadata: {smoke: "sessions", suffix: $suffix}}')")"
  SID="$(echo "$sess_resp" | jq -r '.id // empty')"
  if [[ -z "$SID" ]]; then
    echo "$sess_resp" | jq . >&2
    fail "POST /v1/sessions: id missing"
  fi
  ok "session created: ${SID}"

  log "POST /v1/sessions/${SID}/memories"
  store_resp="$(curl -sf -X POST "${BASE}/v1/sessions/${SID}/memories" \
    "${HDR[@]}" \
    -d "$(jq -n \
      --arg p "$WM_PATH" \
      --arg c "$WM_CONTENT" \
      '{path: $p, content: $c, metadata: {smoke: "sessions"}}')")"
  echo "$store_resp" | jq -e --arg sid "$SID" \
    '.session_id == $sid and .status == "stored" and (.path | length) > 0' >/dev/null
  ok "working memory saved: $(echo "$store_resp" | jq -r '.path')"

  log "GET /v1/sessions/${SID}/memories"
  list_resp="$(curl -sf "${BASE}/v1/sessions/${SID}/memories?limit=10" "${HDR[@]}")"
  echo "$list_resp" | jq -e --arg sid "$SID" --arg content "$WM_CONTENT" \
    '.session_id == $sid and (.total >= 1) and ([.entries[].content] | index($content) != null)' >/dev/null
  ok "session memories: total=$(echo "$list_resp" | jq -r '.total')"

  log "POST /v1/sessions/${SID}/promote"
  promote_resp="$(curl -sf -X POST "${BASE}/v1/sessions/${SID}/promote" \
    "${HDR[@]}" \
    -d '{"target_prefix":"root"}')"
  echo "$promote_resp" | jq -e --arg sid "$SID" \
    '.session_id == $sid and .promoted >= 1 and .status == "promoted"' >/dev/null
  ok "promote: $(echo "$promote_resp" | jq -r '.promoted') righe → $(echo "$promote_resp" | jq -r '.target_prefix')"

  log "DELETE /v1/sessions/${SID}"
  end_resp="$(curl -sf -X DELETE "${BASE}/v1/sessions/${SID}" "${HDR[@]}")"
  echo "$end_resp" | jq -e --arg sid "$SID" \
    '.id == $sid and .status == "ended"' >/dev/null
  ok "session terminata"

  echo ""
  ok "smoke sessions completato"
}

main "$@"
