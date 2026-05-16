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
#   7) Mostra un report finale + log del worker
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
GREEN="\033[1;32m"; YELLOW="\033[1;33m"; RED="\033[1;31m"; BLUE="\033[1;34m"; NC="\033[0m"
log()  { echo -e "${BLUE}[$(date +%H:%M:%S)]${NC} $*"; }
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[ERR]${NC} $*" >&2; }

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
# 4) Baseline counters (active memories + distilled) sul tenant SOC
# =============================================================================
log "Step 4/6 — Baseline counters (tenant=${TENANT_ID})"
BASELINE_STATS="$(curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/stats" || echo '{}')"
BASELINE_ACTIVE="$(echo "$BASELINE_STATS"   | jq -r '.active_memories // 0')"
BASELINE_DISTILLED="$(echo "$BASELINE_STATS" | jq -r '.distilled_count // 0')"
log "  baseline: active_memories=${BASELINE_ACTIVE}  distilled_count=${BASELINE_DISTILLED}"

# =============================================================================
# 5) Genera + ingest + trigger refine
# =============================================================================
log "Step 5/6 — Generazione ${NUM_INCIDENTS} incidenti + ingest"

# --- 5a) Stop worker per evitare la cascata di distillation per ogni store ---
if [[ "$STOP_WORKER_DURING_INGEST" -eq 1 ]]; then
  log "  → stop worker durante l'ingest (evita 1 LLM call per ogni memory.stored)"
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
# Il worker fa LIMIT 100 per refine: 1 refine = 1 distilled (fino a 100 raw memories).
# Per ottenere una distillation significativa partizioniamo per cluster
# semantico (threat_type) e pubblichiamo N refine events. Risultato:
#   N distilled records, ognuno che condensa fino a 100 raw del proprio cluster.
THREAT_TYPES=(phishing brute_force ransomware malware sql_injection insider_threat zero_day ddos)
REFINE_TOTAL=0
for tt in "${THREAT_TYPES[@]}"; do
  PREFIX_TT="${PATH_PREFIX}.${tt}"
  PAYLOAD_TT="$(jq -nc \
    --arg t "$TENANT_ID" \
    --arg p "$PREFIX_TT" \
    --arg r "shard:${tt}" \
    '{Type:"memory.refine.requested", Payload:{tenant_id:$t, path_prefix:$p, reason:$r}}')"
  SUBS="$("${COMPOSE[@]}" exec -T redis redis-cli PUBLISH memory_events "$PAYLOAD_TT" | tr -d '\r\n')"
  REFINE_TOTAL=$((REFINE_TOTAL + 1))
  log "    refine ${tt} → subscribers=${SUBS}"
  # Spacing tra refine: 1 LLM call ≈ 3s con ~1500 tokens; con 8 shard
  # restiamo ampiamente sotto 500 RPM. Bastano 2s.
  sleep 2
done
ok "Pubblicati ${REFINE_TOTAL} refine events (uno per threat_type)"

# --- Sanity post-ingest: verifica che active_memories sia cresciuto ---------
POST_STATS="$(curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/stats" || echo '{}')"
POST_ACTIVE="$(echo "$POST_STATS" | jq -r '.active_memories // 0')"
DELTA_ACTIVE=$(( POST_ACTIVE - BASELINE_ACTIVE ))

if (( DELTA_ACTIVE <= 0 )); then
  err "Ingest reportato OK dal generatore ma /v1/stats.active_memories non è cresciuto"
  err "  baseline=${BASELINE_ACTIVE} → post=${POST_ACTIVE}"
  err "Ecco le ultime righe dell'API per debug:"
  "${COMPOSE[@]}" logs --no-color --tail=60 api || true
  exit 1
else
  ok "active_memories: ${BASELINE_ACTIVE} → ${POST_ACTIVE} (Δ=${DELTA_ACTIVE})"
fi

# =============================================================================
# 6) Verifica distillation
# =============================================================================
if [[ "$SKIP_DISTILL" -eq 1 ]]; then
  warn "--skip-distill: salto la verifica della distillation."
  FINAL_DISTILLED="$BASELINE_DISTILLED"
else
  TARGET_DISTILLED=$(( BASELINE_DISTILLED + REFINE_TOTAL ))
  log "Step 6/6 — Verifica distillation (atteso: distilled_count → ${TARGET_DISTILLED})"
  FINAL_DISTILLED="$BASELINE_DISTILLED"
  for i in $(seq 1 40); do
    FINAL_DISTILLED="$(
      curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/stats" \
      | jq -r '.distilled_count // 0' 2>/dev/null || echo 0
    )"
    if (( FINAL_DISTILLED >= TARGET_DISTILLED )); then
      ok "Distillation completa: distilled_count ${BASELINE_DISTILLED} → ${FINAL_DISTILLED} (poll ${i}/40)"
      break
    fi
    sleep 3
  done

  if (( FINAL_DISTILLED <= BASELINE_DISTILLED )); then
    warn "Nessuna nuova distilled memory rilevata dopo 120s."
    warn "  → docker compose logs worker | tail -100"
    warn "  → OPENAI_API_KEY/DISTILLATION_MODEL potrebbero non essere configurati in .env"
  elif (( FINAL_DISTILLED < TARGET_DISTILLED )); then
    warn "Distillation parziale: ${FINAL_DISTILLED}/${TARGET_DISTILLED}. Alcuni shard potrebbero non avere abbastanza record o aver hit il rate limit."
  fi
fi

# =============================================================================
# Report finale
# =============================================================================
DELTA_DISTILLED=$(( FINAL_DISTILLED - BASELINE_DISTILLED ))
# Sources consumate: ogni distilled record incorpora N source_entry_ids
SOURCES_USED="$(
  curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" \
    "${PCMI_BASE_URL}/v1/distilled?path_prefix=${PATH_PREFIX}&limit=200" \
  | jq '[(.entries // .items // .results // [])[] | (.source_entry_ids // .sources // []) | length] | add // 0'
)"
COVERAGE_PCT=$(awk -v u="${SOURCES_USED:-0}" -v t="$POST_ACTIVE" \
  'BEGIN { if (t==0) print 0; else printf "%.1f", 100.0*u/t }')

echo
echo -e "${GREEN}================== PCMI E2E DISTILLATION TEST — REPORT ==================${NC}"
printf "  API URL              : %s\n" "$PCMI_BASE_URL"
printf "  Tenant SOC           : %s (slug=%s)\n" "$TENANT_ID" "$TENANT_SLUG"
printf "  API key SOC          : %s…\n" "${PCMI_API_KEY:0:14}"
printf "  Seed deterministico  : %s\n" "$SEED"
echo
echo -e "  ${BLUE}── INGEST ──${NC}"
printf "  SOC incidents ingest : %s\n" "$NUM_INCIDENTS"
printf "  Active memories      : %s → %s (Δ=%s)\n" "$BASELINE_ACTIVE" "$POST_ACTIVE" "$DELTA_ACTIVE"
printf "  JSONL backup         : %s\n" "$JSONL_OUT"
echo
echo -e "  ${BLUE}── DISTILLATION ──${NC}"
printf "  Refine events sent   : %s (1 per threat_type)\n" "${REFINE_TOTAL:-0}"
printf "  Distilled records    : %s → %s (Δ=%s)\n" "$BASELINE_DISTILLED" "$FINAL_DISTILLED" "$DELTA_DISTILLED"
printf "  Raw sources used     : %s\n" "$SOURCES_USED"
printf "  Coverage             : %s of %s raw memories (%s%%)\n" "$SOURCES_USED" "$POST_ACTIVE" "$COVERAGE_PCT"
if (( DELTA_DISTILLED > 0 )); then
  RATIO=$(awk -v u="$SOURCES_USED" -v d="$DELTA_DISTILLED" 'BEGIN{printf "%.1f", u/d}')
  printf "  Compression ratio    : %s raw → 1 distilled\n" "$RATIO"
fi
RATE_LIMIT_HITS="$("${COMPOSE[@]}" logs --no-color worker 2>/dev/null | grep -c 'status code: 429' || echo 0)"
printf "  Worker 429 hits      : %s\n" "$RATE_LIMIT_HITS"

# Sample delle distilled (mostra i primi N record con summary + insights)
echo
echo -e "  ${BLUE}── SAMPLE DISTILLED MEMORIES ──${NC}"
curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" \
  "${PCMI_BASE_URL}/v1/distilled?path_prefix=${PATH_PREFIX}&limit=${REFINE_TOTAL:-8}" \
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
log "Full tenant stats (/v1/stats):"
curl -fsS -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/stats" | jq .

# Snippet log worker (per audit)
log "Ultime 30 righe del worker:"
"${COMPOSE[@]}" logs --no-color --tail=30 worker || true

echo -e "${GREEN}=========================================================================${NC}"
ok "End-to-end test completato — funziona se Δ active=${NUM_INCIDENTS}, Δ distilled=${REFINE_TOTAL:-N}, 429 hits=0."
