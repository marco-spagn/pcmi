#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# launch_graph_ui.sh — One-command launch: infrastructure → populate → explore
#
# What it does:
#   1. Starts postgres-age + Redis (or full API+Worker stack)
#   2. Waits for API health
#   3. Generates SOC dataset (if CSV missing)
#   4. Validates the dataset
#   5. Loads nodes + links into PCMI (resumable)
#   6. Prints the UI URL and sample exploration commands
#
# Usage:
#   # ONE COMMAND → stack + SOC data + browser UI (recommended for local test):
#   make demo
#
#   # Same via bash:
#   ENTITY_DEMO=1 bash scripts/e2e/launch_graph_ui.sh
#
#   # Graph only (no LLM extraction / proposals setup):
#   make graph-ui
#
#   # Full stack via docker compose (explicit):
#   FULL_STACK=1 ENTITY_DEMO=1 bash scripts/e2e/launch_graph_ui.sh
#
#   # Faster load (100 incidents instead of 1000):
#   DATASET_SIZE=100 make demo
#
#   # Skip loading (infrastructure only)
#   INFRA_ONLY=1 bash scripts/e2e/launch_graph_ui.sh
#
#   # Choose dataset preset: soc (default) | cti | finance | custom
#   PRESET=soc bash scripts/e2e/launch_graph_ui.sh
#   make demo_2   # multi-CTI JSON from examples/full-cti-dataset/data/
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# ── Config ─────────────────────────────────────────────────────────────────
FULL_STACK="${FULL_STACK:-0}"
INFRA_ONLY="${INFRA_ONLY:-0}"
ENTITY_DEMO="${ENTITY_DEMO:-0}"
DATASET_SIZE="${DATASET_SIZE:-1000}"
PRESET="${PRESET:-soc}"
if [ "$ENTITY_DEMO" = "1" ] && [ "$PRESET" = "soc" ] && [ -z "${PRESET_FORCE:-}" ]; then
  PRESET=cti
fi
CTI_DATA_DIR="${PROJECT_ROOT}/examples/full-cti-dataset/data"
CTI_DATASET_JSON="${CTI_DATASET_JSON:-${CTI_DATA_DIR}/full_cti_dataset.json}"
CTI_VENDOR_REPORTS_JSON="${CTI_VENDOR_REPORTS_JSON:-${CTI_DATA_DIR}/vendor_reports_cti_dataset.json}"
CTI_STIX_BUNDLE_JSON="${CTI_STIX_BUNDLE_JSON:-${CTI_DATA_DIR}/operational_stix_cti_dataset.json}"
CTI_ROOT_KEY="${CTI_ROOT_KEY:-soc_inc_001}"
PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"
AGE_PORT="${AGE_PORT:-5433}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
EXTRACT_LIMIT="${EXTRACT_LIMIT:-12}"
PROPOSE_LIMIT="${PROPOSE_LIMIT:-5}"
OPEN_UI="${OPEN_UI:-1}"
LINK_WORKERS="${LINK_WORKERS:-4}"
export RATE_LIMIT_DISABLED="${RATE_LIMIT_DISABLED:-true}"

# ── Colours ─────────────────────────────────────────────────────────────────
RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'
CYAN=$'\033[0;36m'; BOLD=$'\033[1m'; RESET=$'\033[0m'
info()  { printf "  ${CYAN}→${RESET} %s\n" "$*"; }
ok()    { printf "  ${GREEN}✓${RESET} %s\n" "$*"; }
warn()  { printf "  ${YELLOW}⚠${RESET}  %s\n" "$*"; }
hdr()   { printf "\n${BOLD}━━━ %s ━━━${RESET}\n" "$*"; }
banner(){ printf "${BOLD}%s${RESET}\n" "$*"; }

# ── Prerequisites ───────────────────────────────────────────────────────────
for cmd in docker curl python3 jq; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "${RED}✗ Missing dependency: $cmd${RESET}" >&2
    exit 1
  fi
done
ok "All dependencies present (docker, curl, python3, jq)"

# ── Step 1: Start infrastructure ────────────────────────────────────────────
hdr "Step 1: Start infrastructure"

cd "$PROJECT_ROOT"

if [ "$ENTITY_DEMO" = "1" ]; then
  export EXTRACTION_ENABLED=true
  export LINK_PROPOSALS_ENABLED=true
  export ENTITY_ALIAS_PROPOSALS_ENABLED=true
  info "ENTITY_DEMO=1 — EXTRACTION + LINK_PROPOSALS + ENTITY_ALIAS_PROPOSALS enabled"
  if [ -f "$PROJECT_ROOT/.env" ] && grep -q '^OPENAI_API_KEY=.\+' "$PROJECT_ROOT/.env" 2>/dev/null; then
    ok "OPENAI_API_KEY found in .env"
  else
    warn "OPENAI_API_KEY missing — extraction/proposals will fail until configured in .env"
  fi
fi

GRAPH_COMPOSE=( -f docker-compose.yml -f docker-compose.graph.override.yml --profile graph )

if [ "$FULL_STACK" = "1" ]; then
  info "Starting full stack (postgres-age, redis, api, worker)..."
  docker compose "${GRAPH_COMPOSE[@]}" up -d --build --remove-orphans
  ok "Full stack started"
else
  info "Starting postgres-age, redis, api, worker (graph override)..."
  docker compose "${GRAPH_COMPOSE[@]}" up -d --build postgres-age redis api worker
  ok "Stack started (postgres-age + redis + api + worker)"
fi

# ── Step 2: Wait for health ─────────────────────────────────────────────────
hdr "Step 2: Wait for API health"

info "Waiting for API on ${PCMI_BASE_URL}/v1/ready ..."
for i in $(seq 1 60); do
  if curl -sf "${PCMI_BASE_URL}/v1/ready" >/dev/null 2>&1; then
    ok "API is ready"
    break
  fi
  if [ "$i" = "60" ]; then
    echo "${RED}✗ API did not become healthy within 60s${RESET}" >&2
    echo "Check logs: docker compose logs api | tail -50"
    exit 1
  fi
  sleep 2
done

# Apply Phase C migration on existing postgres-age volumes (initdb only runs once)
apply_age_migrations() {
  local dsn="postgres://pcmi:pcmi@localhost:${AGE_PORT}/pcmi?sslmode=disable"
  if ! command -v psql >/dev/null 2>&1; then
    warn "psql not found — skip migration check (install postgresql client if LLM Propose fails)"
    return 0
  fi
  if ! psql "$dsn" -tAc "SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='graph_link_proposals'" 2>/dev/null | grep -q 1; then
    info "Applying migrations/024_graph_link_proposals.sql on postgres-age..."
    psql "$dsn" -v ON_ERROR_STOP=1 -f "${PROJECT_ROOT}/migrations/024_graph_link_proposals.sql" >/dev/null
    ok "graph_link_proposals table ready"
  fi
}
apply_age_migrations

register_extraction_profiles() {
  info "Registering extraction profiles (soc + realistic graph)…"
  if python3 "${PROJECT_ROOT}/examples/cognitive-graph-entities/register_profiles.py"; then
    ok "Extraction profiles registered"
  else
    warn "Profile registration failed — Extract may fail until profiles are registered"
  fi
}

if [ "$INFRA_ONLY" != "1" ]; then
  register_extraction_profiles
fi

# Verify AGE is available
HEALTH=$(curl -sf "${PCMI_BASE_URL}/v1/graph/health")
AGE_AVAIL=$(echo "$HEALTH" | jq -r '.available')
if [ "$AGE_AVAIL" = "true" ]; then
  ok "Apache AGE is available — graph endpoints active"
else
  warn "Apache AGE not available — /related will use SQL CTE fallback, /chain and /cypher return 501"
fi

# ── Step 3: Generate dataset (if needed) ────────────────────────────────────
hdr "Step 3: Dataset"

if [ "$INFRA_ONLY" = "1" ]; then
  info "INFRA_ONLY=1 — skipping dataset generation and loading"
  hdr "Ready"
  echo ""
  banner "  Infrastructure is up!"
  echo ""
  echo "  Workbench: ${PCMI_BASE_URL}/v1/graph/workbench"
  echo "  Legacy UI: ${PCMI_BASE_URL}/v1/graph/ui"
  echo "  API base:  ${PCMI_BASE_URL}"
  echo "  API key:   ${PCMI_API_KEY}"
  echo ""
  echo "  To load data later:"
  echo "    cd examples/soc-incident-graph && PCMI_BASE_URL=${PCMI_BASE_URL} PCMI_API_KEY=${PCMI_API_KEY} python3 load_to_pcmi.py"
  echo ""
  exit 0
fi

case "$PRESET" in
  soc)
    SOC_DATASET="${PROJECT_ROOT}/examples/soc-incident-graph"
    NODES_CSV="${SOC_DATASET}/soc_incidents_nodes.csv"
    LINKS_CSV="${SOC_DATASET}/soc_incidents_links.csv"
    GENERATOR="${SOC_DATASET}/generate_soc_dataset.py"
    VALIDATOR="${SOC_DATASET}/validate.py"
    LOADER="${SOC_DATASET}/load_to_pcmi.py"
    DEMO_SETUP="${SOC_DATASET}/setup_extraction_demo.py"
    WORKBENCH_MEM="73"
    ;;
  cti)
    CTI_DATASET="${PROJECT_ROOT}/examples/full-cti-dataset"
    VALIDATOR="${CTI_DATASET}/validate.py"
    LOADER="${CTI_DATASET}/load_multi_cti.py"
    DEMO_SETUP="${PROJECT_ROOT}/examples/soc-incident-graph/setup_extraction_demo.py"
    CTI_ALREADY_LOADED=0
    if curl -sf -H "X-API-Key: ${PCMI_API_KEY}" \
      -H "Content-Type: application/json" \
      -d '{"path_prefix":"root.cti","limit":1}' \
      "${PCMI_BASE_URL}/v1/retrieve" 2>/dev/null | jq -e '.entries | length > 0' >/dev/null 2>&1; then
      CTI_ALREADY_LOADED=1
      ok "CTI memories already present under root.cti — skip JSON load"
    fi
    for f in "$CTI_DATASET_JSON" "$CTI_VENDOR_REPORTS_JSON"; do
      if [ ! -f "$f" ] && [ "$CTI_ALREADY_LOADED" != "1" ]; then
        echo "${RED}✗ CTI dataset not found: ${f}${RESET}" >&2
        echo "  Load CTI data first (branch feat/cognitive-graph-ui-v2) or ensure root.cti is populated." >&2
        exit 1
      fi
    done
    if [ ! -f "$CTI_STIX_BUNDLE_JSON" ] && [ "$CTI_ALREADY_LOADED" != "1" ]; then
      warn "STIX bundle missing (${CTI_STIX_BUNDLE_JSON}) — loading SOC + vendor reports only"
    fi
    if [ "$CTI_ALREADY_LOADED" != "1" ]; then
      info "SOC/TI Hub:     ${CTI_DATASET_JSON}"
      info "Vendor reports: ${CTI_VENDOR_REPORTS_JSON}"
      info "STIX bundle:    ${CTI_STIX_BUNDLE_JSON}"
    fi
    export CTI_DATASET_JSON CTI_VENDOR_REPORTS_JSON CTI_STIX_BUNDLE_JSON
    WORKBENCH_MEM=""
    ;;
  *)
    echo "${RED}✗ Unknown preset: $PRESET (supported: soc, cti)${RESET}" >&2
    exit 1
    ;;
esac

if [ "$PRESET" = "soc" ]; then
  CURRENT_NODES=$(wc -l < "$NODES_CSV" 2>/dev/null | tr -d ' ' || echo "0")
  CURRENT_NODES=$((CURRENT_NODES - 1))

  if [ "$CURRENT_NODES" -lt "$DATASET_SIZE" ] 2>/dev/null || [ ! -f "$NODES_CSV" ]; then
    info "Generating SOC dataset (${DATASET_SIZE} nodes)..."
    cd "$SOC_DATASET"
    python3 "$GENERATOR" "$DATASET_SIZE"
    ok "Dataset generated: $(wc -l < "$NODES_CSV" | tr -d ' ') lines (header + nodes)"
  else
    info "Dataset already exists with $CURRENT_NODES nodes — skipping generation"
    info "To regenerate: rm ${NODES_CSV} ${LINKS_CSV} && bash $0"
  fi
else
  info "Using JSON dataset (preset=${PRESET}) — no CSV generation"
fi

# ── Step 4: Validate ────────────────────────────────────────────────────────
hdr "Step 4: Validate dataset"

if [ "$PRESET" = "cti" ] && [ "${CTI_ALREADY_LOADED:-0}" = "1" ]; then
  info "Skipping validate — CTI data already in PCMI"
elif [ -f "$VALIDATOR" ]; then
  cd "$(dirname "$VALIDATOR")"
  if python3 "$VALIDATOR"; then
    ok "Dataset is coherent — 0 errors"
  else
    warn "Validation found issues — loading anyway (check output above)"
  fi
else
  warn "No validator for preset=${PRESET} — skipping"
fi

# ── Step 5: Load into PCMI ──────────────────────────────────────────────────
hdr "Step 5: Load dataset into PCMI"

export PCMI_BASE_URL
export PCMI_API_KEY
export CTI_DATASET_JSON
export CTI_VENDOR_REPORTS_JSON
export CTI_STIX_BUNDLE_JSON
export CTI_ROOT_KEY

if [ "$PRESET" = "cti" ] && [ "${CTI_ALREADY_LOADED:-0}" = "1" ]; then
  info "Skipping load — using existing root.cti memories"
elif [ "$PRESET" = "soc" ]; then
  cd "$SOC_DATASET"
  info "Loading up to ${DATASET_SIZE} nodes + matching links (link-workers=${LINK_WORKERS})..."
  python3 "$LOADER" --batch 50 --link-workers "$LINK_WORKERS" --limit "$DATASET_SIZE"
else
  cd "$CTI_DATASET"
  info "Loading multi-CTI (SOC + vendor reports + STIX)…"
  LOADER_ARGS=(--batch 50 --link-workers "$LINK_WORKERS")
  if [ "${CTI_MULTI_RESET:-0}" = "1" ]; then
    LOADER_ARGS+=(--reset)
  fi
  python3 "$LOADER" "${LOADER_ARGS[@]}"
  if [ -f "${CTI_DATASET}/id_map.json" ]; then
    WORKBENCH_MEM=$(python3 -c "import json,os; m=json.load(open('id_map.json')); print(m.get(os.environ.get('CTI_ROOT_KEY','soc_inc_001'),''))")
  fi
fi

ok "Dataset loaded"

# ── Step 6: Entity extraction demo (optional) ───────────────────────────────
if [ "$ENTITY_DEMO" = "1" ]; then
  hdr "Step 6: Entity extraction + LLM link proposals"
  if [ "$PRESET" = "soc" ]; then
    cd "$SOC_DATASET"
  else
    cd "$CTI_DATASET"
  fi
  info "Running extraction (${EXTRACT_LIMIT}) + proposals (${PROPOSE_LIMIT})…"
  if python3 "$DEMO_SETUP" --extract-limit "$EXTRACT_LIMIT" --propose-limit "$PROPOSE_LIMIT"; then
    ok "Entity demo data ready"
  else
    warn "Entity demo setup had errors — UI still works; run extraction manually"
  fi
fi

# ── Done ────────────────────────────────────────────────────────────────────
hdr "All done!"

if [ -z "$WORKBENCH_MEM" ] && [ -f "${CTI_DATASET:-}/id_map.json" ]; then
  WORKBENCH_MEM=$(python3 -c "import json; m=json.load(open('${CTI_DATASET}/id_map.json')); print(m.get('${CTI_ROOT_KEY}',''))" 2>/dev/null || true)
fi
WORKBENCH_MEM="${WORKBENCH_MEM:-73}"
GRAPH_UI_URL="${PCMI_BASE_URL}/v1/graph/ui?demo=cti&autostart=1"
if RESOLVED=$(PCMI_BASE_URL="$PCMI_BASE_URL" PCMI_API_KEY="$PCMI_API_KEY" \
  python3 "${PROJECT_ROOT}/examples/full-cti-dataset/resolve_demo_ids.py" --url-only 2>/dev/null); then
  GRAPH_UI_URL="${RESOLVED}&autostart=1"
fi

echo ""
banner "  🎉 PCMI Cognitive Graph is ready!"
echo ""
echo "  ┌─────────────────────────────────────────────────────────────┐"
echo "  │  CTI Graph UI: ${GRAPH_UI_URL}  │"
echo "  │  API base:     ${PCMI_BASE_URL}                              │"
echo "  │  API key:      ${PCMI_API_KEY}                                  │"
echo "  └─────────────────────────────────────────────────────────────┘"
echo ""
echo "  Open in browser:"
echo "    ${BOLD}open \"${GRAPH_UI_URL}\"${RESET}"
if [ "$OPEN_UI" = "1" ] && command -v open >/dev/null 2>&1; then
  open "$GRAPH_UI_URL" 2>/dev/null || true
fi
echo ""
if [ "$ENTITY_DEMO" = "1" ]; then
  echo "  CTI demo UI (threat actors · multi-vendor):"
  echo "    • ▶ Tour automatico — 9 passi (banner in basso)"
  echo "    • Colori: teal=SOC · arancio=vendor · viola=STIX"
  echo "    • BRICKSTORM · PRESSURE CHOLLIMA ↔ Sapphire Sleet · PROMPTSTEAL ↔ Forest Blizzard"
  echo "    • View Registry / Proposals — Phase D entity evolution"
  echo ""
fi
echo "  Quick exploration (curl):"
echo ""
echo "    # Health check"
echo "    curl -s ${PCMI_BASE_URL}/v1/graph/health | jq ."
echo ""
echo "    # List first 10 graph memories"
echo "    curl -s -H 'X-API-Key: ${PCMI_API_KEY}' \\"
echo "      '${PCMI_BASE_URL}/v1/graph/memories?limit=10' | jq '.entries[] | {id, path, preview: .preview[:60]}'"
echo ""
echo "    # Traversal from campaign root (memory id=1)"
echo "    curl -s -H 'X-API-Key: ${PCMI_API_KEY}' \\"
echo "      '${PCMI_BASE_URL}/v1/graph/related?memory_id=1&depth=5&link_types=causal,temporal' | jq ."
echo ""
echo "    # Chain reconstruction"
echo "    curl -s -H 'X-API-Key: ${PCMI_API_KEY}' \\"
echo "      '${PCMI_BASE_URL}/v1/graph/chain?from=1&to=9&link_types=causal&max_depth=10' | jq ."
echo ""
echo "  Stop everything:"
echo "    docker compose --profile graph down"
echo ""
