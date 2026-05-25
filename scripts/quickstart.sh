#!/usr/bin/env bash
# PCMI Quickstart — from zero to running distillation in < 3 minutes.
# Usage: bash scripts/quickstart.sh
# Requires: docker, curl, jq

set -euo pipefail

# ── Colors ────────────────────────────────────────────────────────────────────
GREEN=$'\033[0;32m'
YELLOW=$'\033[1;33m'
RED=$'\033[0;31m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
RESET=$'\033[0m'

ok()     { printf "${GREEN}✓${RESET} %s\n" "$*"; }
warn()   { printf "${YELLOW}⚠${RESET}  %s\n" "$*"; }
err()    { printf "${RED}✗${RESET}  %s\n" "$*" >&2; }
info()   { printf "${CYAN}→${RESET} %s\n" "$*"; }
header() { printf "\n${BOLD}%s${RESET}\n" "$*"; }

# ── Spinner ───────────────────────────────────────────────────────────────────
_spinner_pid=""
_spinner_msg=""
spinner_start() {
  _spinner_msg="$1"
  printf "${YELLOW}  %s${RESET}" "$_spinner_msg"
  ( i=0; frames='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    while true; do
      printf '\r'"${YELLOW}%s %s${RESET}" "${frames:$((i % 10)):1}" "$1"
      sleep 0.1
      i=$((i + 1))
    done
  ) &
  _spinner_pid=$!
}
spinner_stop() {
  if [ -n "$_spinner_pid" ]; then
    kill "$_spinner_pid" 2>/dev/null || true
    wait "$_spinner_pid" 2>/dev/null || true
    _spinner_pid=""
    # clear the spinner line completely before the caller prints ok/err
    printf '\r\033[K'
  fi
}

# ── Config ────────────────────────────────────────────────────────────────────
API_URL="${PCMI_API_URL:-http://localhost:8000}"
API_KEY="${PCMI_API_KEY:-testkey123}"
HEALTH_TIMEOUT=60
# Shared header array — same pattern used by every PCMI CI script.
# Using an array avoids quoting/expansion issues inside $() subshells.
hdr=(-H "Content-Type: application/json" -H "X-API-Key: ${API_KEY}")

MEMORIES_STORED=0
MEMORIES_RETRIEVED=0
MEMORIES_DISTILLED=0

# ── Step 1: Dependency checks ─────────────────────────────────────────────────
header "PCMI Quickstart"
info "Checking dependencies..."

missing=()
for dep in docker curl jq; do
  if ! command -v "$dep" &>/dev/null; then
    missing+=("$dep")
  fi
done

if ! docker info &>/dev/null 2>&1; then
  missing+=("docker-daemon (Docker is not running)")
fi

if ! docker compose version &>/dev/null 2>&1; then
  missing+=("docker-compose-v2 (install Docker Desktop or the Compose plugin)")
fi

if [ "${#missing[@]}" -gt 0 ]; then
  err "Missing required dependencies:"
  for m in "${missing[@]}"; do
    printf "    • %s\n" "$m" >&2
  done
  exit 1
fi

ok "All dependencies present (docker, docker compose, curl, jq)"

# ── Step 2: Copy .env.example ─────────────────────────────────────────────────
header "Environment"
if [ ! -f .env ]; then
  if [ ! -f .env.example ]; then
    err ".env.example not found — are you running from the repo root?"
    exit 1
  fi
  cp .env.example .env
  ok "Copied .env.example → .env"
else
  ok ".env already present — skipping copy"
fi

# ── Step 3: docker compose up ─────────────────────────────────────────────────
header "Starting stack"
spinner_start "Building and starting containers (this may take a minute on first run)..."
if ! docker compose up -d --build --wait >/dev/null 2>&1; then
  spinner_stop
  err "docker compose up failed — check: docker compose logs"
  exit 1
fi
spinner_stop
ok "Stack is up"

# ── Step 4: Wait for health ────────────────────────────────────────────────────
header "Health check"
spinner_start "Waiting for API to be ready (max ${HEALTH_TIMEOUT}s)..."
deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
while true; do
  if curl -sf --max-time 3 "${API_URL}/v1/health" 2>/dev/null \
       | jq -e '.status == "ok"' >/dev/null 2>&1; then
    break
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    spinner_stop
    err "API did not become healthy within ${HEALTH_TIMEOUT}s."
    err "Check logs: docker compose logs api"
    exit 1
  fi
  sleep 2
done
spinner_stop
ok "API is healthy at ${API_URL}"

# ── Helper: store a memory ─────────────────────────────────────────────────────
store_memory() {
  local path="$1" content="$2" tags="$3"
  curl -sf --max-time 10 -X POST "${API_URL}/v1/memories" \
    "${hdr[@]}" \
    -d "{\"path\":\"${path}\",\"content\":\"${content}\",\"tags\":${tags}}" \
    >/dev/null
}

# ── Step 5: Store memories ─────────────────────────────────────────────────────
header "Storing memories"

info "SOC scenario — 3 alerts under root.security.alerts"
store_memory "root.security.alerts.brute_force" \
  "Brute-force attempt detected: 2847 failed logins in 30s from 203.0.113.42" \
  '["severity:critical","source:siem","tactic:credential-access"]'
MEMORIES_STORED=$((MEMORIES_STORED + 1))
ok "Stored: root.security.alerts.brute_force"

store_memory "root.security.alerts.lateral_movement" \
  "Lateral movement observed: internal host 10.0.1.55 scanning subnet 10.0.2.0/24 via SMB" \
  '["severity:high","source:ids","tactic:lateral-movement"]'
MEMORIES_STORED=$((MEMORIES_STORED + 1))
ok "Stored: root.security.alerts.lateral_movement"

store_memory "root.security.alerts.exfil_attempt" \
  "Data exfiltration attempt: 4.2 GB outbound to 198.51.100.99 over TLS on port 443" \
  '["severity:critical","source:dlp","tactic:exfiltration"]'
MEMORIES_STORED=$((MEMORIES_STORED + 1))
ok "Stored: root.security.alerts.exfil_attempt"

info "Trading scenario — 2 signals under root.trading.signals"
store_memory "root.trading.signals.btc_breakout" \
  "BTC/USD broke above 200-day MA with 3.2x avg volume; RSI 68, momentum bullish" \
  '["asset:BTC","signal:breakout","confidence:high"]'
MEMORIES_STORED=$((MEMORIES_STORED + 1))
ok "Stored: root.trading.signals.btc_breakout"

store_memory "root.trading.signals.eth_oversold" \
  "ETH/USD RSI touched 28 on 4h chart; historically 73% reversal probability within 48h" \
  '["asset:ETH","signal:oversold","confidence:medium"]'
MEMORIES_STORED=$((MEMORIES_STORED + 1))
ok "Stored: root.trading.signals.eth_oversold"

info "DevOps scenario — 1 incident under root.devops.incidents"
store_memory "root.devops.incidents.db_cpu_spike" \
  "PostgreSQL primary CPU spiked to 95% for 8 min; traced to missing index on audit_logs.tenant_id" \
  '["severity:high","team:platform","status:resolved"]'
MEMORIES_STORED=$((MEMORIES_STORED + 1))
ok "Stored: root.devops.incidents.db_cpu_spike"

printf "\n  Total memories stored: ${GREEN}%d${RESET}\n" "$MEMORIES_STORED"

# ── Step 6: Retrieve memories ─────────────────────────────────────────────────
header "Retrieving memories"
info "Querying root.security for 'critical threat'..."

# Write to a temp file so curl runs outside $() — avoids any subshell
# quoting/expansion issue that could silently drop custom headers.
_ret_tmp=$(mktemp)
curl -s --max-time 10 \
  -X POST "${API_URL}/v1/retrieve" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${API_KEY}" \
  -d '{"path_prefix":"root.security","query":"critical threat","limit":10}' \
  -o "$_ret_tmp" 2>/dev/null || true
response=$(cat "$_ret_tmp" 2>/dev/null || echo '{}')
rm -f "$_ret_tmp"

# Check success by presence of .entries (always in RetrieveResponse),
# not by absence of .error (which is fragile when error is null/"").
if printf '%s' "$response" | jq -e '.entries' >/dev/null 2>&1; then
  MEMORIES_RETRIEVED=$(printf '%s' "$response" | jq '.entries | length' 2>/dev/null || echo 0)
  ok "Retrieved ${MEMORIES_RETRIEVED} memories matching 'critical threat' under root.security"
  if [ "$MEMORIES_RETRIEVED" -gt 0 ]; then
    printf '%s' "$response" \
      | jq -r '.entries[]? | "    • \(.path): \(.content | .[0:80])…"' 2>/dev/null || true
  fi
else
  _errmsg=$(printf '%s' "$response" | jq -r '.error // "no response"' 2>/dev/null || echo "no response")
  warn "Retrieve failed: ${_errmsg}"
  warn "Manual check: curl -s -X POST ${API_URL}/v1/retrieve -H 'X-API-Key: ${API_KEY}' -H 'Content-Type: application/json' -d '{\"path_prefix\":\"root.security\",\"limit\":1}' | jq ."
fi

# ── Step 7 & 8: Distillation ──────────────────────────────────────────────────
header "Distillation"
# The worker's fallback timer polls root.test every ~15s. Store a couple of
# memories there so distillation runs without needing a pre-existing DB policy.
info "Storing distillation seed memories under root.test (worker fallback path)..."
store_memory "root.test.quickstart.sec1" \
  "Quickstart SOC seed: brute-force and lateral-movement alerts require immediate triage" \
  '["quickstart","scenario:security"]'
store_memory "root.test.quickstart.sec2" \
  "Quickstart SOC seed: data exfiltration over TLS on 443 requires DLP rule and IOC block" \
  '["quickstart","scenario:security"]'
MEMORIES_STORED=$((MEMORIES_STORED + 2))
ok "Stored 2 seed memories under root.test for distillation"

info "Polling /v1/distilled?path_prefix=root.test (max 60s, worker fires every ~15s)..."
spinner_start "Waiting for distilled insights..."
deadline=$(( $(date +%s) + 60 ))
while true; do
  dist_result=$(curl -sf --max-time 5 \
    "${API_URL}/v1/distilled?path_prefix=root.test&limit=5" \
    "${hdr[@]}" 2>/dev/null) || dist_result='{}'
  MEMORIES_DISTILLED=$(printf '%s' "$dist_result" | jq '.entries | length // 0' 2>/dev/null || echo 0)
  if [ "$MEMORIES_DISTILLED" -gt 0 ] || [ "$(date +%s)" -ge "$deadline" ]; then
    break
  fi
  sleep 5
done
spinner_stop

if [ "$MEMORIES_DISTILLED" -gt 0 ]; then
  ok "Found ${MEMORIES_DISTILLED} distilled insight(s):"
  printf '%s' "$dist_result" \
    | jq -r '.entries[]? | "    • \(.path): \(.content // .summary | .[0:100])…"' 2>/dev/null || true
else
  warn "No distilled results yet — worker needs OPENAI_API_KEY in .env to run distillation."
  warn "Set OPENAI_API_KEY=sk-... in .env, restart the stack, and re-run."
fi

# ── Step 9: Summary table ──────────────────────────────────────────────────────
header "Summary"
printf "  %-25s %s\n" "Metric" "Value"
printf "  %-25s %s\n" "─────────────────────" "───────"
printf "  %-25s ${GREEN}%d${RESET}\n" "Memories stored"     "$MEMORIES_STORED"
printf "  %-25s ${GREEN}%d${RESET}\n" "Memories retrieved"  "$MEMORIES_RETRIEVED"
printf "  %-25s ${GREEN}%d${RESET}\n" "Distilled insights"  "$MEMORIES_DISTILLED"

# ── Step 10: Next steps ────────────────────────────────────────────────────────
header "Next steps"
cat <<EOF
  ${CYAN}Install the Python SDK:${RESET}
    pip install pcmi-sdk
    export PCMI_BASE_URL="${API_URL}"
    export PCMI_API_KEY="${API_KEY}"

  ${CYAN}Install the TypeScript SDK:${RESET}
    npm install @pcmi/sdk

  ${CYAN}Use gRPC (high-throughput):${RESET}
    grpcurl -H "X-API-Key: ${API_KEY}" -plaintext localhost:50051 list

  ${CYAN}Read the docs:${RESET}
    docs/USAGE.md       — full API reference
    docs/SESSIONS.md    — agent sessions
    make infra-smoke    — verify all endpoints

  ${CYAN}Stop the stack:${RESET}
    docker compose down

EOF

ok "Quickstart complete — PCMI is running at ${API_URL}"
exit 0
