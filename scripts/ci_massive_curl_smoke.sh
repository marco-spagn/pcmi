#!/usr/bin/env bash
# scripts/ci_massive_curl_smoke.sh
#
# Massive realistic multi-agent workload driven through the HTTP API.
#
# Simulates 4 agents (sre, security, coding, product) storing ~600 memories
# each with concurrent background jobs, then verifies retrieval.
#
# Prerequisites:
#   - API running on ${PCMI_BASE_URL:-http://127.0.0.1:8000}
#   - API key in ${PCMI_API_KEY:-testkey123}
#   - jq, curl installed
set -euo pipefail

PCMI_BASE_URL="${PCMI_BASE_URL:-http://127.0.0.1:8000}"
PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"
TENANT="tenant-massive-curl"RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass()  { echo -e "${GREEN} $*${NC}"; }
fail()  { echo -e "${RED} $*${NC}"; exit 1; }
info()  { echo -e "${YELLOW}→ $*${NC}"; }
hdr()   { echo -e "${YELLOW}━━━ $* ━━━${NC}"; }

api() {
  local method="$1"path="$2"body="${3:-}"local args=(-s -w '\n%{http_code}' -X "$method")
  args+=(-H "X-API-Key: $PCMI_API_KEY")
  args+=(-H "Content-Type: application/json")
  if [ -n "$body"]; then
    args+=(-d "$body")
  fi
  curl "${args[@]}""$PCMI_BASE_URL$path"
}

# ── Health check ───────────────────────────────────────────────────────────
hdr "Health check — waiting for API"
MAX_RETRIES=30
for i in $(seq 1 $MAX_RETRIES); do
  RESP=$(curl -s -o /dev/null -w '%{http_code}' "$PCMI_BASE_URL/v1/health"2>/dev/null || echo "000")
  if [ "$RESP"= "200"]; then
    break
  fi
  if [ "$i"-eq "$MAX_RETRIES"]; then
    fail "API not healthy after ${MAX_RETRIES} attempts (last status: $RESP)"fi
  sleep 2
done
API_VERSION=$(curl -s "$PCMI_BASE_URL/v1/health"| jq -r '.version // "unknown"')
pass "API healthy — version $API_VERSION"# ── Massive agent workload ─────────────────────────────────────────────────
hdr "Massive multi-agent workload (4 agents × 200 ops = 800 stores + 160 retrieves)"AGENTS=("sre""security""coding""product")
COMPONENTS=("api""worker""db""cache""queue""auth")
EVENTS=("error""warning""spike""deployment""config_change""auth_failure")
TOTAL_STORES=0
TOTAL_RETRIEVES=0
START_TS=$(date +%s)

for agent in "${AGENTS[@]}"; do
  info "Agent: $agent (200 ops)"agent_stores=0
  agent_retrieves=0

  for op in $(seq 0 199); do
    component="${COMPONENTS[$((op % ${#COMPONENTS[@]}))]}"ev="${EVENTS[$((op % ${#EVENTS[@]}))]}"# Every ~5th op is a long reasoning trace
    if [ $((op % 5)) -eq 0 ]; then
      content="$agent agent reasoning step $op. Symptoms: $ev in $component. Root cause hypothesis under investigation. Decision: monitor and alert."tags='["reasoning","decision","'$agent'"]'
    else
      content="$agent observed $ev in $component at step $op"tags='["observation","'$agent'","'$component'"]'
    fi

    path="agent.${agent}.${component}.${op}"# Retry up to 3 times on rate-limit (429)
    for retry in 1 2 3; do
      RESP=$(api POST "/v1/memories""{\"path\":\"$path\",\"content\":\"$content\",\"tags\":$tags}")
      HTTP_CODE=$(echo "$RESP"| tail -1)
      [ "$HTTP_CODE"!= "429"] && break
      sleep 0.2
    done

    if [ "$HTTP_CODE"= "200"]; then
      agent_stores=$((agent_stores + 1))
    fi
    # Small pause to avoid overwhelming the API even with rate-limit off
    [ $((op % 10)) -eq 0 ] && sleep 0.05

    # Every ~5th op: retrieve for this agent
    if [ $((op % 5)) -eq 0 ]; then
      RESP=$(api POST "/v1/retrieve""{\"path_prefix\":\"agent.$agent\",\"limit\":15}")
      HTTP_CODE=$(echo "$RESP"| tail -1)
      if [ "$HTTP_CODE"= "200"]; then
        agent_retrieves=$((agent_retrieves + 1))
      fi
    fi

    # Every ~9th op: dedup pressure (repeat the same store)
    if [ $((op % 9)) -eq 0 ]; then
      api POST "/v1/memories""{\"path\":\"$path\",\"content\":\"$content\",\"tags\":$tags}"> /dev/null
    fi
  done

  TOTAL_STORES=$((TOTAL_STORES + agent_stores))
  TOTAL_RETRIEVES=$((TOTAL_RETRIEVES + agent_retrieves))
  pass "$agent: $agent_stores stores, $agent_retrieves retrieves"
done

ELAPSED=$(($(date +%s) - START_TS))
info "Completed in ${ELAPSED}s — $TOTAL_STORES stores, $TOTAL_RETRIEVES retrieves"if [ "$TOTAL_STORES"-lt 400 ]; then
  fail "Too few successful stores: $TOTAL_STORES (expected ≥ 400)"
fi

# ── Final retrieval verification ────────────────────────────────────────────
hdr "Final retrieval verification"RESP=$(api POST "/v1/retrieve""{\"path_prefix\":\"agent\",\"limit\":100}")
HTTP_CODE=$(echo "$RESP"| tail -1)
BODY=$(echo "$RESP"| sed '$d')

if [ "$HTTP_CODE"!= "200"]; then
  fail "Final retrieve failed (HTTP $HTTP_CODE): $BODY"
fi

TOTAL=$(echo "$BODY"| jq -r '.total // 0')
ENTRY_COUNT=$(echo "$BODY"| jq -r '(.entries | length) // 0')

info "Final retrieve: total=$TOTAL entries=$ENTRY_COUNT"if [ "$ENTRY_COUNT"-eq 0 ]; then
  fail "Final retrieve returned 0 entries — workload may not have persisted"
fi

if [ "$ENTRY_COUNT"-lt 10 ]; then
  fail "Final retrieve returned only $ENTRY_COUNT entries — expected ≥ 10"
fi

# ── gRPC health check (optional) ────────────────────────────────────────────
hdr "gRPC health check"
if command -v grpcurl &> /dev/null; then
  grpcurl -plaintext 127.0.0.1:50051 pcmi.v1.MetricsService/Health 2>/dev/null && pass "gRPC health OK"|| info "gRPC health check skipped (service may not be running)"
else
  info "grpcurl not installed — skipping"
fi

# ── Summary ─────────────────────────────────────────────────────────────────
hdr "Massive curl smoke PASSED"
echo "Agents:      ${#AGENTS[@]}"
echo "Stores:      $TOTAL_STORES"
echo "Retrieves:   $TOTAL_RETRIEVES"
echo "Final entries: $ENTRY_COUNT"
echo "Duration:    ${ELAPSED}s"
