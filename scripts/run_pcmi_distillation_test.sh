#!/usr/bin/env bash
# =============================================================================
# run_pcmi_distillation_test.sh
# -----------------------------------------------------------------------------
# End-to-end test of the PCMI Distillation Pipeline:
#   1) Start infrastructure via docker compose (postgres + redis + api + worker)
#   2) Wait for API and worker to be healthy
#   3) Install the PCMI Python SDK (editable) in a local venv
#   4) Run `generate_soc_incidents_enterprise_v2.py` (100 incidents default, seed 42)
#   5) Publish the Redis event `memory.refine.requested`
#   6) Verify that the worker has generated distilled memories (GET /v1/distilled)
#   7) Final report: for each metric show x → y with expected z (OK if y == z)
#
# Verification model (x → y, expected z):
#   - active_memories:     x=baseline,  y=after ingest,  z=x+NUM_INCIDENTS
#   - distilled_count:     x=baseline,  y=after refine,  z=x+NUM_REFINE_EVENTS
#   - Redis refine events: x=0,         y=published,     z=ceil(NUM_INCIDENTS/10) (one per shard)
#   - raw in distilled:    sum source_entry_ids ≈ NUM_INCIDENTS
#
# Idempotent: can be re-run as many times as needed.
#
# Usage:
#   ./scripts/run_pcmi_distillation_test.sh                 # full e2e
#   ./scripts/run_pcmi_distillation_test.sh --no-build      # skip `docker compose build`
#   ./scripts/run_pcmi_distillation_test.sh --no-teardown   # leave containers running
#   ./scripts/run_pcmi_distillation_test.sh --preset finance --num 500 --seed 1
#   ./scripts/run_pcmi_distillation_test.sh --num 200       # fewer records (default preset soc)
#   ./scripts/run_pcmi_distillation_test.sh --llm --domain "retail fraud cases"
#   ./scripts/run_pcmi_distillation_test.sh --skip-distill  # skip final check
#
# Useful env:
#   PCMI_API_KEY   (default: testkey123, seeded in migration 003)
#   PCMI_BASE_URL  (default: http://localhost:8000)
#   PCMI_REDIS_URL (default: redis://localhost:6379/0)
#   EVENT_BACKEND  (default streams) — script publishes to pcmi:events (XADD) or memory_events (PUBLISH)
#   DISTILLATION_POLICY_DISABLED  (default 1 for this script) — no auto-distill on memory.stored
# =============================================================================

set -Eeuo pipefail

# --- Config ------------------------------------------------------------------
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Same .env used by docker compose (OPENAI_API_KEY, EVENT_BACKEND, ...)
if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi
EVENT_BACKEND="${EVENT_BACKEND:-streams}"
# Smoke: distillation only via memory.refine.requested (not policy / stored backlog)
DISTILLATION_POLICY_DISABLED="${DISTILLATION_POLICY_DISABLED:-1}"
export DISTILLATION_POLICY_DISABLED

PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
# Admin key seeded via migration 003 → default tenant 00000000-...
ADMIN_API_KEY="${PCMI_ADMIN_API_KEY:-testkey123}"
# Bootstrap: PCMI_API_KEY starts as admin key, then gets reassigned
# to the dedicated SOC tenant key after provisioning (Step 3.5).
PCMI_API_KEY="${PCMI_API_KEY:-${ADMIN_API_KEY}}"
PCMI_REDIS_URL="${PCMI_REDIS_URL:-redis://localhost:6379/0}"

# Dedicated SOC test tenant: created with forced UUID via direct SQL,
# then an admin API key is generated for it via /v1/admin/api-keys.
PRESET="${PRESET:-soc}"
TENANT_ID="${TENANT_ID:-a1b2c3d4-e5f6-7890-abcd-ef1234567890}"
TENANT_SLUG="${TENANT_SLUG:-}"

NUM_INCIDENTS=1000
SEED=42
USE_LLM=0
CUSTOM_DOMAIN=""
SKIP_BUILD=0
SKIP_TEARDOWN=0
SKIP_DISTILL=0
# DISTILLATION_BATCH_SIZE (LIMIT of the worker query). Changeable via flag
# --distill-batch-size or env DISTILLATION_BATCH_SIZE.
# The generator's shard size is kept in sync with this value so
# each refine covers exactly one complete shard.
DISTILL_BATCH_SIZE="${DISTILLATION_BATCH_SIZE:-10}"
# Throttle: practically irrelevant if we stop the worker during ingest,
# because memory.stored events are never consumed (Redis pub/sub
# is not persistent). Kept low just to avoid breaking the DB pool.
THROTTLE_MS=0
BATCH_SIZE=50
PATH_PREFIX=""
# "no LLM cascade" strategy: stop worker during ingest, restart only for
# the single manual refine event → 1 total LLM call.
STOP_WORKER_DURING_INGEST=1

OUTPUT_DIR="${ROOT}/.pcmi_test_out"
JSONL_OUT=""
WORKER_LOG="${OUTPUT_DIR}/worker.log"
API_LOG="${OUTPUT_DIR}/api.log"

VENV="${ROOT}/.venv_e2e"

# --- ANSI / logging ----------------------------------------------------------
GREEN="\033[1;32m"; YELLOW="\033[1;33m"; RED="\033[1;31m"; BLUE="\033[1;34m"; CYAN="\033[1;36m"; NC="\033[0m"
log()  { echo -e "${BLUE}[$(date +%H:%M:%S)]${NC} $*"; }
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[ERR]${NC} $*" >&2; }

TEST_FAILED=0

# Sharding: each shard has DISTILL_BATCH_SIZE records (= LIMIT of the worker query).
# Number of refine events = ceil(NUM_INCIDENTS / DISTILL_BATCH_SIZE).
# Examples:
#   1000 incidents / batch=10  → 100 shards → 100 distilled
#   1000 incidents / batch=100 →  10 shards →  10 distilled (with richer summaries)
#   1000 incidents / batch=50  →  20 shards →  20 distilled
SHARD_SIZE="$DISTILL_BATCH_SIZE"
NUM_REFINE_EVENTS=$(( (NUM_INCIDENTS + SHARD_SIZE - 1) / SHARD_SIZE ))

fetch_active_memories() {
  curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/stats" \
    | jq -r '.active_memories // 0' 2>/dev/null || echo 0
}

fetch_distilled_count() {
  curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/stats" \
    | jq -r '.distilled_count // 0' 2>/dev/null || echo 0
}

fetch_sources_used() {
  curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" \
    "${PCMI_BASE_URL}/v1/distilled?path_prefix=${PATH_PREFIX}&limit=200" \
  | jq '[(.entries // .items // .results // [])[] | (.source_entry_ids // .sources // []) | length] | add // 0' 2>/dev/null || echo 0
}

# grep -c with 0 matches prints "0" but exits 1: do NOT use "|| echo 0" (produces "0\n0").
count_worker_429() {
  local n
  n=$("${COMPOSE[@]}" logs --no-color worker 2>/dev/null | grep -c 'status code: 429' || true)
  echo "${n:-0}"
}

# Publishes a PCMI event on the configured backend (streams = default from worker).
publish_memory_event() {
  local payload="$1"
  if [[ "${EVENT_BACKEND}" == "pubsub" ]]; then
    "${COMPOSE[@]}" exec -T redis redis-cli PUBLISH memory_events "$payload" >/dev/null
    return 0
  fi
  local etype data
  etype=$(echo "$payload" | jq -r '.Type')
  data=$(echo "$payload" | jq -c .)
  printf '%s' "$data" | "${COMPOSE[@]}" exec -T -i redis redis-cli -x XADD pcmi:events '*' type "$etype" data >/dev/null
}

# After ingest with worker stopped, the API has XADDed memory.stored to the stream: flush before refine.
flush_event_stream_backlog() {
  if [[ "${EVENT_BACKEND}" == "pubsub" ]]; then
    return 0
  fi
  log "  → DEL pcmi:events (removes memory.stored backlog from ingest)"
  "${COMPOSE[@]}" exec -T redis redis-cli DEL pcmi:events >/dev/null 2>&1 || true
}

# Print metric row: start x, expected z, (optional) current value y
metric_row() {
  local label="$1" x="$2" z="$3" y="${4:-—}"
  local mark=" "
  if [[ "$y" != "—" && "$y" == "$z" ]]; then mark="${GREEN}${NC}"
  elif [[ "$y" != "—" && "$y" != "$z" ]]; then mark="${RED}${NC}"
  fi
  printf "  ${mark} %-28s  x=%-6s  →  y=%-6s  (expected z=%s)\n" "$label" "$x" "$y" "$z"
}

# Verify y == z; increment TEST_FAILED if different
assert_eq() {
  local label="$1" y="$2" z="$3" hint="${4:-}"
  if [[ "$y" == "$z" ]]; then
    ok "${label}: y=${y} == z=${z} (OK)"
    return 0
  fi
  err "${label}: y=${y} ≠ z=${z} (FAIL)${hint:+ — $hint}"
  TEST_FAILED=1
  return 1
}

print_test_plan() {
  local x_active="$1" x_distilled="$2"
  local z_active=$(( x_active + NUM_INCIDENTS ))
  local z_distilled=$(( x_distilled + NUM_REFINE_EVENTS ))
  echo
  echo -e "${CYAN}══════════════════════════════════════════════════════════════════════${NC}"
  echo -e "${CYAN}  TEST PLAN — each row: start x → final value y, expected z${NC}"
  echo -e "${CYAN}══════════════════════════════════════════════════════════════════════${NC}"
  printf "  %-28s  x=%-6s  →  y=???     (expected z=%s)\n" "Raw memories (active_memories)" "$x_active" "$z_active"
  printf "  %-28s  x=%-6s  →  y=???     (expected z=%s)\n" "Distilled records (distilled_count)" "$x_distilled" "$z_distilled"
  printf "  %-28s  x=%-6s  →  y=???     (expected z=%s)\n" "Redis refine events published" "0" "$NUM_REFINE_EVENTS"
  printf "  %-28s  x=%-6s  →  y=???     (expected z=%s)\n" "Synthetic records generated" "0" "$NUM_INCIDENTS"
  if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
    printf "  %-28s  x=%-6s  →  y=???     (expected z=%s)\n" "memory.stored events consumed" "0" "0"
    echo "      (worker stopped during ingest: stores do not trigger automatic distillation)"
  fi
  echo -e "${CYAN}──────────────────────────────────────────────────────────────────────${NC}"
  echo "  Redis events we will send: ${NUM_REFINE_EVENTS}× memory.refine.requested"
  echo "    (one per shard: shard_000..shard_$(printf "%03d" $((NUM_REFINE_EVENTS-1))), ${SHARD_SIZE} records/shard)"
  echo
}

# --- CLI ---------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build)     SKIP_BUILD=1; shift ;;
    --no-teardown)  SKIP_TEARDOWN=1; shift ;;
    --skip-distill) SKIP_DISTILL=1; shift ;;
    --num)          NUM_INCIDENTS="${2:?missing N}"
                    NUM_REFINE_EVENTS=$(( (NUM_INCIDENTS + SHARD_SIZE - 1) / SHARD_SIZE ))
                    shift 2 ;;
    --seed)         SEED="${2:?missing seed}"; shift 2 ;;
    --throttle-ms)  THROTTLE_MS="${2:?missing ms}"; shift 2 ;;
    --batch-size)   BATCH_SIZE="${2:?missing N}"; shift 2 ;;
    --distill-batch-size)
                    DISTILL_BATCH_SIZE="${2:?missing N}"
                    SHARD_SIZE="$DISTILL_BATCH_SIZE"
                    NUM_REFINE_EVENTS=$(( (NUM_INCIDENTS + SHARD_SIZE - 1) / SHARD_SIZE ))
                    shift 2 ;;
    --preset)       PRESET="${2:?missing preset}"; shift 2 ;;
    --llm)          USE_LLM=1; shift ;;
    --domain)       CUSTOM_DOMAIN="${2:?missing domain}"; shift 2 ;;
    --path-prefix)  PATH_PREFIX="${2:?missing prefix}"; shift 2 ;;
    --fast)         THROTTLE_MS=0; shift ;;
    --keep-worker)  STOP_WORKER_DURING_INGEST=0; shift ;;
    --help|-h)
      sed -n '2,30p' "$0"; exit 0 ;;
    *) err "Unknown flag: $1"; exit 2 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"

# --- Preset defaults (path + tenant slug) ------------------------------------
_apply_preset_defaults() {
  case "$PRESET" in
    soc)
      PATH_PREFIX="${PATH_PREFIX:-root.security.incidents.soc}"
      TENANT_SLUG="${TENANT_SLUG:-soc-test}"
      ;;
    finance)
      PATH_PREFIX="${PATH_PREFIX:-root.finance.events}"
      TENANT_SLUG="${TENANT_SLUG:-finance-test}"
      ;;
    advertising)
      PATH_PREFIX="${PATH_PREFIX:-root.marketing.ads}"
      TENANT_SLUG="${TENANT_SLUG:-ads-test}"
      ;;
    healthcare)
      PATH_PREFIX="${PATH_PREFIX:-root.healthcare.ops}"
      TENANT_SLUG="${TENANT_SLUG:-health-test}"
      ;;
    custom)
      PATH_PREFIX="${PATH_PREFIX:-root.custom.synthetic}"
      TENANT_SLUG="${TENANT_SLUG:-custom-test}"
      USE_LLM=1
      ;;
    *)
      err "Unknown --preset ${PRESET} (soc|finance|advertising|healthcare|custom)"
      exit 2
      ;;
  esac
  JSONL_OUT="${JSONL_OUT:-${OUTPUT_DIR}/${PRESET}_seed${SEED}_n${NUM_INCIDENTS}.jsonl}"
}
_apply_preset_defaults

if [[ "$PRESET" == "custom" && -z "$CUSTOM_DOMAIN" ]]; then
  err "preset=custom requires --domain \"...\" (used with --llm)"
  exit 2
fi

# --- Preflight ---------------------------------------------------------------
need() { command -v "$1" >/dev/null 2>&1 || { err "Missing tool: $1"; exit 1; }; }
need docker
need curl
need jq
need python3

COMPOSE=(docker compose)
if ! docker compose version >/dev/null 2>&1; then
  if command -v docker-compose >/dev/null 2>&1; then
    COMPOSE=(docker-compose)
  else
    err "Need 'docker compose' or 'docker-compose' in PATH"
    exit 1
  fi
fi

# --- Teardown trap -----------------------------------------------------------
cleanup() {
  local ec=$?
  if [[ "$SKIP_TEARDOWN" -eq 1 ]]; then
    warn "--no-teardown: containers left running."
    return
  fi
  log "Tearing down docker compose stack…"
  "${COMPOSE[@]}" logs --no-color worker > "$WORKER_LOG" 2>&1 || true
  "${COMPOSE[@]}" logs --no-color api    > "$API_LOG"    2>&1 || true
  "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  exit "$ec"
}
trap cleanup EXIT INT TERM

# =============================================================================
# 1) Bootstrap infra
# =============================================================================
log "Step 1/6 — Bootstrap infrastructure (postgres + redis + api + worker)"

if [[ "$SKIP_BUILD" -eq 0 ]]; then
  log "  docker compose build (api + worker)…"
  "${COMPOSE[@]}" build >/dev/null
fi

# Idempotent stop of leftover containers
"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true

log "  docker compose up -d (DISTILLATION_BATCH_SIZE=${DISTILL_BATCH_SIZE}, EVENT_BACKEND=${EVENT_BACKEND}, DISTILLATION_POLICY_DISABLED=${DISTILLATION_POLICY_DISABLED})…"
export DISTILLATION_BATCH_SIZE="$DISTILL_BATCH_SIZE"
"${COMPOSE[@]}" up -d >/dev/null

# =============================================================================
# 2) Health wait
# =============================================================================
log "Step 2/6 — Waiting for healthcheck (postgres, redis, api)"

wait_url() {
  local url="$1" tries="${2:-60}" name="${3:-endpoint}"
  for i in $(seq 1 "$tries"); do
    if curl -fsS -m 2 -H "X-API-Key: ${PCMI_API_KEY}" "$url" >/dev/null 2>&1; then
      ok "$name healthy ($url) after ${i}s"
      return 0
    fi
    sleep 1
  done
  err "Timeout waiting for $name at $url"
  return 1
}

# Postgres readiness via container healthcheck
for i in $(seq 1 60); do
  if [[ "$("${COMPOSE[@]}" ps --format json postgres 2>/dev/null \
          | jq -r 'if type=="array" then .[0].Health else .Health end' 2>/dev/null)" == "healthy" ]]; then
    ok "postgres healthy"
    break
  fi
  sleep 1
done

wait_url "${PCMI_BASE_URL}/v1/health" 90 "PCMI API"

# Sanity: worker process up (its container is running)
if [[ "$("${COMPOSE[@]}" ps --format json worker 2>/dev/null \
        | jq -r 'if type=="array" then .[0].State else .State end' 2>/dev/null)" == "running" ]]; then
  ok "worker container running"
else
  warn "worker container not in 'running' state — distillation may not trigger"
fi

# =============================================================================
# 3) Setup Python venv + install SDK
# =============================================================================
log "Step 3/6 — Python venv + install PCMI SDK + redis"
if [[ ! -d "$VENV" ]]; then
  python3 -m venv "$VENV"
fi
# shellcheck disable=SC1091
source "${VENV}/bin/activate"
python -m pip install -q --upgrade pip
# install SDK in editable + redis client async
python -m pip install -q -e "${ROOT}/sdk/python" "redis>=5.0,<6"
ok "PCMI SDK + redis installed in ${VENV}"

# =============================================================================
# 3.5) Provisioning tenant SOC dedicato + API key admin per quel tenant
# =============================================================================
log "Step 3.5/6 — Provisioning tenant ${TENANT_ID} + dedicated API key"

# (a) create the tenant with forced UUID via direct SQL on compose postgres,
#     because the admin/tenants endpoint generates a random UUID.
"${COMPOSE[@]}" exec -T postgres psql -U pcmi -d pcmi -v ON_ERROR_STOP=1 -q <<SQL
INSERT INTO tenants (id, slug, name, settings)
VALUES ('${TENANT_ID}', '${TENANT_SLUG}', 'SOC Test Tenant', '{}'::jsonb)
ON CONFLICT (id) DO NOTHING;
SQL
ok "tenant ${TENANT_SLUG} (${TENANT_ID}) present"

# (b) create (or reuse) an admin API key for that tenant.
#     Reuse: look for an existing key with name='soc-bulk-loader'.
KEY_NAME="soc-bulk-loader"
EXISTING_KEY_ID="$(
  curl -fsS -H "X-API-Key: ${ADMIN_API_KEY}" \
    "${PCMI_BASE_URL}/v1/admin/api-keys?tenant_id=${TENANT_ID}&limit=200" \
  | jq -r --arg n "$KEY_NAME" '(.api_keys // []) | map(select(.name==$n)) | .[0].id // empty'
)"

if [[ -n "$EXISTING_KEY_ID" ]]; then
  log "  found existing API key (id=${EXISTING_KEY_ID}) → rotate to get plaintext back"
  ROTATE_JSON="$(curl -fsS -X POST -H "X-API-Key: ${ADMIN_API_KEY}" -H "Content-Type: application/json" \
    -d "$(jq -nc --arg n "$KEY_NAME" '{name:$n}')" \
    "${PCMI_BASE_URL}/v1/admin/api-keys/${EXISTING_KEY_ID}/rotate")"
  PCMI_API_KEY="$(echo "$ROTATE_JSON" | jq -r '.api_key')"
else
  log "  creating admin API key for tenant"
  CREATE_JSON="$(curl -fsS -X POST -H "X-API-Key: ${ADMIN_API_KEY}" -H "Content-Type: application/json" \
    -d "$(jq -nc --arg t "$TENANT_ID" --arg n "$KEY_NAME" '{tenant_id:$t, name:$n, role:"admin"}')" \
    "${PCMI_BASE_URL}/v1/admin/api-keys")"
  PCMI_API_KEY="$(echo "$CREATE_JSON" | jq -r '.api_key')"
fi

if [[ -z "$PCMI_API_KEY" || "$PCMI_API_KEY" == "null" ]]; then
  err "Unable to obtain API key for ${TENANT_ID}"
  exit 1
fi
ok "API key for SOC tenant: ${PCMI_API_KEY:0:14}..."
export PCMI_API_KEY

# =============================================================================
# 4) Baseline (x values) for SOC tenant
# =============================================================================
log "Step 4/6 — Initial values x (tenant=${TENANT_ID})"
X_ACTIVE="$(fetch_active_memories)"
X_DISTILLED="$(fetch_distilled_count)"
EXPECT_ACTIVE=$(( X_ACTIVE + NUM_INCIDENTS ))
EXPECT_DISTILLED=$(( X_DISTILLED + NUM_REFINE_EVENTS ))
metric_row "active_memories (x)" "$X_ACTIVE" "$EXPECT_ACTIVE"
metric_row "distilled_count (x)" "$X_DISTILLED" "$EXPECT_DISTILLED"
print_test_plan "$X_ACTIVE" "$X_DISTILLED"

# =============================================================================
# 5) Genera + ingest + trigger refine
# =============================================================================
log "Step 5/6 — Ingest ${NUM_INCIDENTS} records (preset=${PRESET}, seed=${SEED}; expected active_memories: x=${X_ACTIVE} → z=${EXPECT_ACTIVE})"

# --- 5a) Stop worker to avoid distillation cascade on every store ---
if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
  log "  → stop worker: ${NUM_INCIDENTS} HTTP stores without consuming memory.stored events on Redis"
  "${COMPOSE[@]}" stop worker >/dev/null
fi

# --- 5b) Ingest puro (no redis publish lato Python) -------------------------
SYNTH_ARGS=(
  generate
  --preset "$PRESET"
  --num "$NUM_INCIDENTS"
  --seed "$SEED"
  --tenant-id "$TENANT_ID"
  --api-url "$PCMI_BASE_URL"
  --api-key "$PCMI_API_KEY"
  --output "$JSONL_OUT"
  --path-prefix "$PATH_PREFIX"
  --batch-size "$BATCH_SIZE"
  --throttle-ms "$THROTTLE_MS"
  --shard-size "$SHARD_SIZE"
)
if [[ "$USE_LLM" -eq 1 ]]; then
  SYNTH_ARGS+=(--llm)
  [[ -n "$CUSTOM_DOMAIN" ]] && SYNTH_ARGS+=(--domain "$CUSTOM_DOMAIN")
fi
PYTHONPATH="${ROOT}/scripts" python3 -m pcmi_synth "${SYNTH_ARGS[@]}"

ok "Generator completed — JSONL backup: $JSONL_OUT"

# --- Immediate ingest check: active_memories x → y, expected z ---
Y_ACTIVE="$(fetch_active_memories)"
DELTA_ACTIVE=$(( Y_ACTIVE - X_ACTIVE ))
metric_row "active_memories (post-ingest)" "$X_ACTIVE" "$EXPECT_ACTIVE" "$Y_ACTIVE"
assert_eq "active_memories after ingest" "$Y_ACTIVE" "$EXPECT_ACTIVE" \
  "generator wrote ${NUM_INCIDENTS} records; check API logs" || {
  "${COMPOSE[@]}" logs --no-color --tail=60 api || true
}

# --- 5c) Restart worker: flush stream backlog, then wait for consumer ---
if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
  flush_event_stream_backlog
  log "  → start worker to process refine events"
  "${COMPOSE[@]}" start worker >/dev/null
  for i in $(seq 1 30); do
    if [[ "${EVENT_BACKEND}" == "pubsub" ]]; then
      if "${COMPOSE[@]}" logs --no-color --tail=80 worker 2>/dev/null \
           | grep -qE 'Redis connected|event backend=pubsub'; then
        ok "worker ready (pubsub, attempt ${i}/30)"
        break
      fi
    elif "${COMPOSE[@]}" logs --no-color --tail=80 worker 2>/dev/null \
         | grep -q "Subscribed to stream pcmi:events"; then
      ok "worker subscribed to stream pcmi:events (attempt ${i}/30)"
      break
    fi
    sleep 1
  done
fi

# --- 5d) Publish ONE refine event PER SHARD (path: ...soc.shard_NNN) ---
# Worker does LIMIT 10 per refine. Publishing 1 refine per shard gives
# FULL COVERAGE of raw incidents (1000 → 100 distilled).
# Rate-limit gpt-4o-mini = 500 RPM / 200k TPM, each call ≈ 1500 tokens:
# spacing 1.2s ≈ 50 RPM, ~75k TPM → ample margin.
log "  → publishing ${NUM_REFINE_EVENTS} Redis memory.refine.requested events"
log "     (expected distilled_count: x=${X_DISTILLED} → z=${EXPECT_DISTILLED})"
REFINE_PUBLISHED=0
for ((sh=0; sh<NUM_REFINE_EVENTS; sh++)); do
  PREFIX_SH="$(printf "%s.shard_%03d" "$PATH_PREFIX" "$sh")"
  PAYLOAD_SH="$(jq -nc \
    --arg t "$TENANT_ID" \
    --arg p "$PREFIX_SH" \
    --arg r "shard_$(printf "%03d" "$sh")" \
    '{Type:"memory.refine.requested", Payload:{tenant_id:$t, path_prefix:$p, reason:$r}}')"
  publish_memory_event "$PAYLOAD_SH"
  REFINE_PUBLISHED=$((REFINE_PUBLISHED + 1))
  if (( REFINE_PUBLISHED % 10 == 0 )); then
    log "    refine published: ${REFINE_PUBLISHED}/${NUM_REFINE_EVENTS}"
  fi
  sleep 1.2
done
ok "Published ${REFINE_PUBLISHED} refine events"

# =============================================================================
# 6) Verify distillation: distilled_count x → y, expected z
# =============================================================================
if [[ "$SKIP_DISTILL" -eq 1 ]]; then
  warn "--skip-distill: skipping distilled_count check."
  Y_DISTILLED="$X_DISTILLED"
else
  log "Step 6/6 — Waiting for distillation (x=${X_DISTILLED} → z=${EXPECT_DISTILLED}, ${NUM_REFINE_EVENTS} LLM jobs in parallel)"
  Y_DISTILLED="$X_DISTILLED"
  for i in $(seq 1 40); do
    Y_DISTILLED="$(fetch_distilled_count)"
    if (( Y_DISTILLED >= EXPECT_DISTILLED )); then
      ok "distilled_count reached z=${EXPECT_DISTILLED} at poll ${i}/40 (y=${Y_DISTILLED})"
      break
    fi
    log "  poll ${i}/40: distilled_count y=${Y_DISTILLED} (expected z=${EXPECT_DISTILLED})..."
    sleep 3
  done
  metric_row "distilled_count" "$X_DISTILLED" "$EXPECT_DISTILLED" "$Y_DISTILLED"
  assert_eq "distilled_count after refine" "$Y_DISTILLED" "$EXPECT_DISTILLED" \
    "OPENAI_API_KEY in .env? docker compose logs worker | tail -80"
fi

# =============================================================================
# Final report — summary x → y (expected z), OK if y == z
# =============================================================================
EXPECT_SOURCES="$NUM_INCIDENTS"
Y_SOURCES="$(fetch_sources_used)"
COVERAGE_PCT=$(awk -v u="${Y_SOURCES:-0}" -v t="$Y_ACTIVE" \
  'BEGIN { if (t==0) print 0; else printf "%.1f", 100.0*u/t }')
RATE_LIMIT_HITS="$(count_worker_429)"
EXPECT_RATE_LIMIT=0

echo
echo -e "${GREEN}══════════════════════ SUMMARY x → y (expected z) ══════════════════════${NC}"
echo "  Legend: x=initial value, y=final value, z=expected. Test OK if y == z."
echo
metric_row "Raw memories (active_memories)" "$X_ACTIVE" "$EXPECT_ACTIVE" "$Y_ACTIVE"
metric_row "Distilled records (distilled_count)" "$X_DISTILLED" "$EXPECT_DISTILLED" "$Y_DISTILLED"
metric_row "Redis refine events sent" "0" "$NUM_REFINE_EVENTS" "$REFINE_PUBLISHED"
metric_row "Raw referenced in distilled" "0" "$EXPECT_SOURCES" "$Y_SOURCES"
metric_row "OpenAI rate-limit errors (429)" "0" "$EXPECT_RATE_LIMIT" "$RATE_LIMIT_HITS"
echo
echo -e "  ${BLUE}── EVENTS (what happened) ──${NC}"
printf "  • %s POST batch → %s raw memories in DB (path under %s.*)\n" "$NUM_INCIDENTS" "$NUM_INCIDENTS" "$PATH_PREFIX"
if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
  printf "  • Worker stopped during ingest: ~%s memory.stored events on Redis not consumed\n" "$NUM_INCIDENTS"
fi
printf "  • %s× PUBLISH memory.refine.requested → worker starts %s LLM jobs (1 distilled/shard)\n" \
  "$NUM_REFINE_EVENTS" "$NUM_REFINE_EVENTS"
printf "  • Coverage raw→distilled: %s / %s (%.1f%%)\n" "$Y_SOURCES" "$Y_ACTIVE" "$COVERAGE_PCT"
printf "  • JSONL backup: %s\n" "$JSONL_OUT"
echo

# Verifiche finali (fail esplicito)
assert_eq "active_memories (finale)" "$Y_ACTIVE" "$EXPECT_ACTIVE"
assert_eq "distilled_count (finale)" "$Y_DISTILLED" "$EXPECT_DISTILLED"
assert_eq "refine events published" "$REFINE_PUBLISHED" "$NUM_REFINE_EVENTS"
assert_eq "raw sources in distilled" "$Y_SOURCES" "$EXPECT_SOURCES" \
  "each shard contains exactly ${SHARD_SIZE} records → each distilled has ${SHARD_SIZE} source_entry_ids"
assert_eq "worker 429 hits" "$RATE_LIMIT_HITS" "$EXPECT_RATE_LIMIT"

# Sample delle distilled
echo
echo -e "  ${BLUE}── SAMPLE DISTILLED (y per shard) ──${NC}"
curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" \
  "${PCMI_BASE_URL}/v1/distilled?path_prefix=${PATH_PREFIX}&limit=${NUM_REFINE_EVENTS}" \
| jq -r '
    (.entries // .items // .results // [])
    | .[]
    | "
  • path : \(.path)
    sources: \((.source_entry_ids // .sources // []) | length)
    summary: \((.summary // .content // "")[0:240])\((.summary // .content // "" | if length > 240 then "…" else "" end))
    insights: \((.insights // []) | if type == "array" then map(tostring) | join(" | ") else tostring end)"
  ' 2>/dev/null || true

echo
echo -e "  ${BLUE}── PER-SHARD SOURCES (from API) ──${NC}"
curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" \
  "${PCMI_BASE_URL}/v1/distilled?path_prefix=${PATH_PREFIX}&limit=200" \
| jq -r '
    (.entries // .items // .results // [])
    | sort_by(.path)
    | .[]
    | "  \(.path): \((.source_entry_ids // .sources // []) | length) raw sources"
  ' 2>/dev/null || true

echo
log "Stats tenant (/v1/stats):"
curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/stats" | jq .

log "Last 30 worker lines:"
"${COMPOSE[@]}" logs --no-color --tail=30 worker || true

echo
echo -e "${GREEN}══════════════════════════════════════════════════════════════════════${NC}"
if [[ "$TEST_FAILED" -eq 0 ]]; then
  ok "TEST PASS — all metrics: y == z"
  echo "  active_memories: ${X_ACTIVE} → ${Y_ACTIVE} (z=${EXPECT_ACTIVE})"
  echo "  distilled_count: ${X_DISTILLED} → ${Y_DISTILLED} (z=${EXPECT_DISTILLED})"
  echo "  refine events:   0 → ${REFINE_PUBLISHED} (z=${NUM_REFINE_EVENTS})"
else
  err "TEST FAIL — at least one metric has y ≠ z (see  rows above)"
  exit 1
fi
echo -e "${GREEN}══════════════════════════════════════════════════════════════════════${NC}"
