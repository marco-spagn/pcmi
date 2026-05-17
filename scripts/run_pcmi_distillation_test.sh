#!/usr/bin/env bash
# =============================================================================
# run_pcmi_distillation_test.sh
# -----------------------------------------------------------------------------
# End-to-end test del PCMI Distillation Pipeline:
#   1) Avvia l'infrastruttura via docker compose (postgres + redis + api + worker)
#   2) Aspetta che API e worker siano healthy
#   3) Installa il PCMI Python SDK (in editable) in un venv locale
#   4) Lancia `generate_soc_incidents_enterprise_v2.py` (100 incidenti default, seed 42)
#   5) Pubblica l'evento Redis `memory.refine.requested`
#   6) Verifica che il worker abbia generato distilled memories (GET /v1/distilled)
#   7) Report finale: per ogni metrica mostra x → y con atteso z (OK se y == z)
#
# Modello di verifica (x → y, atteso z):
#   - active_memories:     x=baseline,  y=dopo ingest,  z=x+NUM_INCIDENTS
#   - distilled_count:     x=baseline,  y=dopo refine,  z=x+NUM_REFINE_EVENTS
#   - eventi Redis refine: x=0,         y=pubblicati,   z=8 (uno per threat_type)
#   - raw in distillati:   somma source_entry_ids ≈ NUM_INCIDENTS
#
# Idempotente: si può rilanciare quante volte si vuole.
#
# Uso:
#   ./scripts/run_pcmi_distillation_test.sh                 # full e2e
#   ./scripts/run_pcmi_distillation_test.sh --no-build      # salta `docker compose build`
#   ./scripts/run_pcmi_distillation_test.sh --no-teardown   # lascia i container su
#   ./scripts/run_pcmi_distillation_test.sh --num 200       # genera meno incidenti
#   ./scripts/run_pcmi_distillation_test.sh --skip-distill  # salta il check finale
#
# Env utili:
#   PCMI_API_KEY   (default: testkey123, seedata nella migration 003)
#   PCMI_BASE_URL  (default: http://localhost:8000)
#   PCMI_REDIS_URL (default: redis://localhost:6379/0)
# =============================================================================

set -Eeuo pipefail

# --- Config ------------------------------------------------------------------
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
# Admin key seedata via migration 003 → tenant default 00000000-…
ADMIN_API_KEY="${PCMI_ADMIN_API_KEY:-testkey123}"
# Bootstrap: PCMI_API_KEY parte come admin key, poi viene riassegnato
# alla key dedicata del tenant SOC dopo il provisioning (Step 3.5).
PCMI_API_KEY="${PCMI_API_KEY:-${ADMIN_API_KEY}}"
PCMI_REDIS_URL="${PCMI_REDIS_URL:-redis://localhost:6379/0}"

# Tenant dedicato per il test SOC: lo creiamo con UUID forzato via SQL,
# poi generiamo una API key admin per lui via /v1/admin/api-keys.
TENANT_ID="${TENANT_ID:-a1b2c3d4-e5f6-7890-abcd-ef1234567890}"
TENANT_SLUG="${TENANT_SLUG:-soc-test}"

NUM_INCIDENTS=100
SEED=42
SKIP_BUILD=0
SKIP_TEARDOWN=0
SKIP_DISTILL=0
# Throttle: praticamente irrilevante se fermiamo il worker durante l'ingest,
# perché gli eventi memory.stored non vengono mai consumati (Redis pub/sub
# non è persistente). Lo manteniamo basso giusto per non rompere il pool DB.
THROTTLE_MS=0
BATCH_SIZE=50
PATH_PREFIX="root.security.incidents.soc"
# Strategia "no LLM cascade": stop worker durante l'ingest, riparte solo per
# il singolo refine event manuale → 1 sola chiamata LLM totale.
STOP_WORKER_DURING_INGEST=1

OUTPUT_DIR="${ROOT}/.pcmi_test_out"
JSONL_OUT="${OUTPUT_DIR}/soc_incidents_backup.jsonl"
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

# Threat shards (1 refine event Redis ciascuno → 1 distilled record)
THREAT_TYPES=(phishing brute_force ransomware malware sql_injection insider_threat zero_day ddos)
NUM_REFINE_EVENTS="${#THREAT_TYPES[@]}"

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

# grep -c con 0 match stampa "0" ma exit 1: NON usare "|| echo 0" (produce "0\n0").
count_worker_429() {
  local n
  n=$("${COMPOSE[@]}" logs --no-color worker 2>/dev/null | grep -c 'status code: 429' || true)
  echo "${n:-0}"
}

# Stampa riga metrica: partenza x, atteso z, (opzionale) valore attuale y
metric_row() {
  local label="$1" x="$2" z="$3" y="${4:-—}"
  local mark=" "
  if [[ "$y" != "—" && "$y" == "$z" ]]; then mark="${GREEN}✓${NC}"
  elif [[ "$y" != "—" && "$y" != "$z" ]]; then mark="${RED}✗${NC}"
  fi
  printf "  ${mark} %-28s  x=%-6s  →  y=%-6s  (atteso z=%s)\n" "$label" "$x" "$y" "$z"
}

# Verifica y == z; incrementa TEST_FAILED se diverso
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
  echo -e "${CYAN}  PIANO TEST — ogni riga: partenza x → valore finale y, atteso z${NC}"
  echo -e "${CYAN}══════════════════════════════════════════════════════════════════════${NC}"
  printf "  %-28s  x=%-6s  →  y=???     (atteso z=%s)\n" "Memorie raw (active_memories)" "$x_active" "$z_active"
  printf "  %-28s  x=%-6s  →  y=???     (atteso z=%s)\n" "Record distillati (distilled_count)" "$x_distilled" "$z_distilled"
  printf "  %-28s  x=%-6s  →  y=???     (atteso z=%s)\n" "Eventi Redis refine pubblicati" "0" "$NUM_REFINE_EVENTS"
  printf "  %-28s  x=%-6s  →  y=???     (atteso z=%s)\n" "Incidenti SOC generati/ingest" "0" "$NUM_INCIDENTS"
  if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
    printf "  %-28s  x=%-6s  →  y=???     (atteso z=%s)\n" "Eventi memory.stored consumati" "0" "0"
    echo "      (worker fermo in ingest: gli store non triggerano distillazione automatica)"
  fi
  echo -e "${CYAN}──────────────────────────────────────────────────────────────────────${NC}"
  echo "  Eventi Redis che invieremo: ${NUM_REFINE_EVENTS}× memory.refine.requested"
  echo "    (uno per threat_type: ${THREAT_TYPES[*]})"
  echo
}

# --- CLI ---------------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build)     SKIP_BUILD=1; shift ;;
    --no-teardown)  SKIP_TEARDOWN=1; shift ;;
    --skip-distill) SKIP_DISTILL=1; shift ;;
    --num)          NUM_INCIDENTS="${2:?missing N}"; shift 2 ;;
    --seed)         SEED="${2:?missing seed}"; shift 2 ;;
    --throttle-ms)  THROTTLE_MS="${2:?missing ms}"; shift 2 ;;
    --batch-size)   BATCH_SIZE="${2:?missing N}"; shift 2 ;;
    --fast)         THROTTLE_MS=0; shift ;;
    --keep-worker)  STOP_WORKER_DURING_INGEST=0; shift ;;
    --help|-h)
      sed -n '2,30p' "$0"; exit 0 ;;
    *) err "Unknown flag: $1"; exit 2 ;;
  esac
done

mkdir -p "$OUTPUT_DIR"

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
log "Step 1/6 — Bootstrap infrastruttura (postgres + redis + api + worker)"

if [[ "$SKIP_BUILD" -eq 0 ]]; then
  log "  docker compose build (api + worker)…"
  "${COMPOSE[@]}" build >/dev/null
fi

# Stop residui idempotente
"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true

log "  docker compose up -d…"
"${COMPOSE[@]}" up -d >/dev/null

# =============================================================================
# 2) Health wait
# =============================================================================
log "Step 2/6 — Attesa healthcheck (postgres, redis, api)"

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
  warn "worker container not in 'running' state — distillation potrebbe non triggerare"
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
ok "PCMI SDK + redis installati in ${VENV}"

# =============================================================================
# 3.5) Provisioning tenant SOC dedicato + API key admin per quel tenant
# =============================================================================
log "Step 3.5/6 — Provisioning tenant ${TENANT_ID} + API key dedicata"

# (a) crea il tenant con UUID forzato via SQL diretto sul postgres del compose,
#     perché l'endpoint admin/tenants genera UUID random.
"${COMPOSE[@]}" exec -T postgres psql -U pcmi -d pcmi -v ON_ERROR_STOP=1 -q <<SQL
INSERT INTO tenants (id, slug, name, settings)
VALUES ('${TENANT_ID}', '${TENANT_SLUG}', 'SOC Test Tenant', '{}'::jsonb)
ON CONFLICT (id) DO NOTHING;
SQL
ok "tenant ${TENANT_SLUG} (${TENANT_ID}) presente"

# (b) crea (o riusa) una API key admin per quel tenant.
#     Riuso: cerco una chiave esistente con name='soc-bulk-loader'.
KEY_NAME="soc-bulk-loader"
EXISTING_KEY_ID="$(
  curl -fsS -H "X-API-Key: ${ADMIN_API_KEY}" \
    "${PCMI_BASE_URL}/v1/admin/api-keys?tenant_id=${TENANT_ID}&limit=200" \
  | jq -r --arg n "$KEY_NAME" '(.api_keys // []) | map(select(.name==$n)) | .[0].id // empty'
)"

if [[ -n "$EXISTING_KEY_ID" ]]; then
  log "  trovata API key esistente (id=${EXISTING_KEY_ID}) → rotate per riavere la plaintext"
  ROTATE_JSON="$(curl -fsS -X POST -H "X-API-Key: ${ADMIN_API_KEY}" -H "Content-Type: application/json" \
    -d "$(jq -nc --arg n "$KEY_NAME" '{name:$n}')" \
    "${PCMI_BASE_URL}/v1/admin/api-keys/${EXISTING_KEY_ID}/rotate")"
  PCMI_API_KEY="$(echo "$ROTATE_JSON" | jq -r '.api_key')"
else
  log "  creo API key admin per il tenant"
  CREATE_JSON="$(curl -fsS -X POST -H "X-API-Key: ${ADMIN_API_KEY}" -H "Content-Type: application/json" \
    -d "$(jq -nc --arg t "$TENANT_ID" --arg n "$KEY_NAME" '{tenant_id:$t, name:$n, role:"admin"}')" \
    "${PCMI_BASE_URL}/v1/admin/api-keys")"
  PCMI_API_KEY="$(echo "$CREATE_JSON" | jq -r '.api_key')"
fi

if [[ -z "$PCMI_API_KEY" || "$PCMI_API_KEY" == "null" ]]; then
  err "Impossibile ottenere l'API key per ${TENANT_ID}"
  exit 1
fi
ok "API key per tenant SOC: ${PCMI_API_KEY:0:14}…"
export PCMI_API_KEY

# =============================================================================
# 4) Baseline (valori x) sul tenant SOC
# =============================================================================
log "Step 4/6 — Valori iniziali x (tenant=${TENANT_ID})"
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
log "Step 5/6 — Ingest ${NUM_INCIDENTS} incidenti (atteso active_memories: x=${X_ACTIVE} → z=${EXPECT_ACTIVE})"

# --- 5a) Stop worker per evitare la cascata di distillation per ogni store ---
if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
  log "  → stop worker: ${NUM_INCIDENTS} store HTTP senza consumare eventi memory.stored su Redis"
  "${COMPOSE[@]}" stop worker >/dev/null
fi

# --- 5b) Ingest puro (no redis publish lato Python) -------------------------
python "${ROOT}/scripts/generate_soc_incidents_enterprise_v2.py" \
  --num-incidents "$NUM_INCIDENTS" \
  --tenant-id     "$TENANT_ID" \
  --api-url       "$PCMI_BASE_URL" \
  --api-key       "$PCMI_API_KEY" \
  --seed          "$SEED" \
  --output        "$JSONL_OUT" \
  --redis-url     "$PCMI_REDIS_URL" \
  --refine-path-prefix "$PATH_PREFIX" \
  --batch-size    "$BATCH_SIZE" \
  --throttle-ms   "$THROTTLE_MS" \
  --skip-publish

ok "Generatore completato — JSONL backup: $JSONL_OUT"

# --- Verifica ingest subito: active_memories x → y, atteso z ---
Y_ACTIVE="$(fetch_active_memories)"
DELTA_ACTIVE=$(( Y_ACTIVE - X_ACTIVE ))
metric_row "active_memories (post-ingest)" "$X_ACTIVE" "$EXPECT_ACTIVE" "$Y_ACTIVE"
assert_eq "active_memories dopo ingest" "$Y_ACTIVE" "$EXPECT_ACTIVE" \
  "generatore ha scritto ${NUM_INCIDENTS} record; controlla log API" || {
  "${COMPOSE[@]}" logs --no-color --tail=60 api || true
}

# --- 5c) Riavvio worker e attesa che sia sottoscritto a Redis ----------------
if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
  log "  → start worker per processare il refine event"
  "${COMPOSE[@]}" start worker >/dev/null
  for i in $(seq 1 30); do
    if "${COMPOSE[@]}" logs --no-color --tail=50 worker 2>/dev/null \
         | grep -q "subscribing to memory_events"; then
      ok "worker risubscribed a memory_events (tentativo ${i}/30)"
      break
    fi
    sleep 1
  done
fi

# --- 5d) Publish UN refine event PER OGNI threat_type ------------------------
# Il worker fa LIMIT 100 per refine: 1 refine = 1 distilled (fino a 100 raw/shard).
log "  → pubblicazione ${NUM_REFINE_EVENTS} eventi Redis memory.refine.requested"
log "     (atteso distilled_count: x=${X_DISTILLED} → z=${EXPECT_DISTILLED})"
REFINE_PUBLISHED=0
for tt in "${THREAT_TYPES[@]}"; do
  PREFIX_TT="${PATH_PREFIX}.${tt}"
  PAYLOAD_TT="$(jq -nc \
    --arg t "$TENANT_ID" \
    --arg p "$PREFIX_TT" \
    --arg r "shard:${tt}" \
    '{Type:"memory.refine.requested", Payload:{tenant_id:$t, path_prefix:$p, reason:$r}}')"
  SUBS="$("${COMPOSE[@]}" exec -T redis redis-cli PUBLISH memory_events "$PAYLOAD_TT" | tr -d '\r\n')"
  REFINE_PUBLISHED=$((REFINE_PUBLISHED + 1))
  log "    evento ${REFINE_PUBLISHED}/${NUM_REFINE_EVENTS} memory.refine.requested  shard=${tt}  subscribers=${SUBS}"
  # Spacing tra refine: 1 LLM call ≈ 3s con ~1500 tokens; con 8 shard
  # restiamo ampiamente sotto 500 RPM. Bastano 2s.
  sleep 2
done

# =============================================================================
# 6) Verifica distillation: distilled_count x → y, atteso z
# =============================================================================
if [[ "$SKIP_DISTILL" -eq 1 ]]; then
  warn "--skip-distill: salto verifica distilled_count."
  Y_DISTILLED="$X_DISTILLED"
else
  log "Step 6/6 — Attesa distillazione (x=${X_DISTILLED} → z=${EXPECT_DISTILLED}, ${NUM_REFINE_EVENTS} job LLM in parallelo)"
  Y_DISTILLED="$X_DISTILLED"
  for i in $(seq 1 40); do
    Y_DISTILLED="$(fetch_distilled_count)"
    if (( Y_DISTILLED >= EXPECT_DISTILLED )); then
      ok "distilled_count raggiunto z=${EXPECT_DISTILLED} al poll ${i}/40 (y=${Y_DISTILLED})"
      break
    fi
    log "  poll ${i}/40: distilled_count y=${Y_DISTILLED} (atteso z=${EXPECT_DISTILLED})…"
    sleep 3
  done
  metric_row "distilled_count" "$X_DISTILLED" "$EXPECT_DISTILLED" "$Y_DISTILLED"
  assert_eq "distilled_count dopo refine" "$Y_DISTILLED" "$EXPECT_DISTILLED" \
    "OPENAI_API_KEY in .env? docker compose logs worker | tail -80"
fi

# =============================================================================
# Report finale — riepilogo x → y (atteso z), OK se y == z
# =============================================================================
EXPECT_SOURCES="$NUM_INCIDENTS"
Y_SOURCES="$(fetch_sources_used)"
COVERAGE_PCT=$(awk -v u="${Y_SOURCES:-0}" -v t="$Y_ACTIVE" \
  'BEGIN { if (t==0) print 0; else printf "%.1f", 100.0*u/t }')
RATE_LIMIT_HITS="$(count_worker_429)"
EXPECT_RATE_LIMIT=0

echo
echo -e "${GREEN}══════════════════════ RIEPILOGO x → y (atteso z) ══════════════════════${NC}"
echo "  Legenda: x=valore iniziale, y=valore finale, z=atteso. Test OK se y == z."
echo
metric_row "Memorie raw (active_memories)" "$X_ACTIVE" "$EXPECT_ACTIVE" "$Y_ACTIVE"
metric_row "Record distillati (distilled_count)" "$X_DISTILLED" "$EXPECT_DISTILLED" "$Y_DISTILLED"
metric_row "Eventi Redis refine inviati" "0" "$NUM_REFINE_EVENTS" "$REFINE_PUBLISHED"
metric_row "Raw referenziati nei distillati" "0" "$EXPECT_SOURCES" "$Y_SOURCES"
metric_row "Errori rate-limit OpenAI (429)" "0" "$EXPECT_RATE_LIMIT" "$RATE_LIMIT_HITS"
echo
echo -e "  ${BLUE}── EVENTI (cosa è successo) ──${NC}"
printf "  • %s POST batch → %s memorie raw in DB (path sotto %s.*)\n" "$NUM_INCIDENTS" "$NUM_INCIDENTS" "$PATH_PREFIX"
if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
  printf "  • Worker spento in ingest: ~%s eventi memory.stored su Redis non consumati\n" "$NUM_INCIDENTS"
fi
printf "  • %s× PUBLISH memory.refine.requested → worker avvia %s job LLM (1 distilled/shard)\n" \
  "$NUM_REFINE_EVENTS" "$NUM_REFINE_EVENTS"
printf "  • Coverage raw→distillati: %s / %s (%.1f%%)\n" "$Y_SOURCES" "$Y_ACTIVE" "$COVERAGE_PCT"
printf "  • JSONL backup: %s\n" "$JSONL_OUT"
echo

# Verifiche finali (fail esplicito)
assert_eq "active_memories (finale)" "$Y_ACTIVE" "$EXPECT_ACTIVE"
assert_eq "distilled_count (finale)" "$Y_DISTILLED" "$EXPECT_DISTILLED"
assert_eq "refine events pubblicati" "$REFINE_PUBLISHED" "$NUM_REFINE_EVENTS"
assert_eq "raw sources nei distillati" "$Y_SOURCES" "$EXPECT_SOURCES" \
  "ogni incidente dovrebbe finire in un solo shard threat_type"
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

log "Ultime 30 righe del worker:"
"${COMPOSE[@]}" logs --no-color --tail=30 worker || true

echo
echo -e "${GREEN}══════════════════════════════════════════════════════════════════════${NC}"
if [[ "$TEST_FAILED" -eq 0 ]]; then
  ok "TEST PASS — tutte le metriche: y == z"
  echo "  active_memories: ${X_ACTIVE} → ${Y_ACTIVE} (z=${EXPECT_ACTIVE})"
  echo "  distilled_count: ${X_DISTILLED} → ${Y_DISTILLED} (z=${EXPECT_DISTILLED})"
  echo "  refine events:   0 → ${REFINE_PUBLISHED} (z=${NUM_REFINE_EVENTS})"
else
  err "TEST FAIL — almeno una metrica ha y ≠ z (vedi righe ✗ sopra)"
  exit 1
fi
echo -e "${GREEN}══════════════════════════════════════════════════════════════════════${NC}"
