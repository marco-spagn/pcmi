#!/usr/bin/env bash
# run_all_tests.sh — lancia unit test, stress test e integration test in sequenza.
#
# Uso:
#   ./scripts/run_all_tests.sh                  # tutto (avvia Postgres via Docker)
#   ./scripts/run_all_tests.sh --unit-only       # solo unit + stress (niente Docker)
#   ./scripts/run_all_tests.sh --integration-only # solo integration (Postgres già attivo)
#   ./scripts/run_all_tests.sh --no-cleanup      # lascia Postgres girare al termine
#   PCMI_GO_TEST_P=2 ./scripts/run_all_tests.sh  # limita parallelismo pacchetti
#
# Output colorato. Exit code 1 se almeno una fase fallisce.
#
# Prerequisiti: go, docker (per integration). curl/jq opzionali.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ── opzioni ──────────────────────────────────────────────────────────────────
UNIT_ONLY=0
INTEGRATION_ONLY=0
NO_CLEANUP=0
for arg in "$@"; do
  case "$arg" in
    --unit-only)         UNIT_ONLY=1 ;;
    --integration-only)  INTEGRATION_ONLY=1 ;;
    --no-cleanup)        NO_CLEANUP=1 ;;
    -h|--help)
      sed -n '2,11p' "$0"
      exit 0
      ;;
    *) echo "Argomento sconosciuto: $arg" >&2; exit 2 ;;
  esac
done

# ── colori ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

PASS=0; FAIL=0; SKIP=0
FAILED_ITEMS=()         # raccoglie etichetta + sezione di ogni fallimento
CURRENT_SECTION="?"     # aggiornato da section()

section() {
  CURRENT_SECTION="$1"
  echo -e "\n${CYAN}${BOLD}══ $1 ══${RESET}"
}
ok()   { echo -e "  ${GREEN}✓${RESET} $1"; PASS=$((PASS+1)); }
fail() {
  echo -e "  ${RED}✗${RESET} $1" >&2
  FAIL=$((FAIL+1))
  FAILED_ITEMS+=("[${CURRENT_SECTION}] $1")
}
skip() { echo -e "  ${YELLOW}⊘${RESET} $1"; SKIP=$((SKIP+1)); }

# ── variabili ────────────────────────────────────────────────────────────────
DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
GT_P="${PCMI_GO_TEST_P:-}"
GT_TIMEOUT="${PCMI_GO_TEST_TIMEOUT:-15m}"
POSTGRES_STARTED_BY_US=0

GT_FLAGS="-race -count=1 -timeout ${GT_TIMEOUT}"
[ -n "$GT_P" ] && GT_FLAGS="$GT_P $GT_FLAGS"

# ── cleanup on exit ───────────────────────────────────────────────────────────
on_exit() {
  local code=$?
  if [ "$POSTGRES_STARTED_BY_US" -eq 1 ] && [ "$NO_CLEANUP" -eq 0 ]; then
    echo -e "\n${YELLOW}→ fermo Postgres avviato da questo script…${RESET}"
    $DOCKER_COMPOSE stop postgres redis >/dev/null 2>&1 || true
  fi
  echo -e "\n${BOLD}══ RIEPILOGO ══${RESET}"
  echo -e "  ${GREEN}✓ passati${RESET}:  $PASS"
  echo -e "  ${RED}✗ falliti${RESET}:  $FAIL"
  echo -e "  ${YELLOW}⊘ saltati${RESET}:  $SKIP"

  if [ "${#FAILED_ITEMS[@]}" -gt 0 ]; then
    echo -e "\n${RED}${BOLD}╔══ COSA È FALLITO ══╗${RESET}"
    for item in "${FAILED_ITEMS[@]}"; do
      echo -e "  ${RED}✗${RESET}  $item"
    done
    echo -e "${RED}${BOLD}╚════════════════════╝${RESET}"
    echo -e "\n  Suggerimento: rilancia solo i fallimenti con:"
    echo -e "  ${CYAN}DATABASE_URL=... go test -race -count=1 -tags integration -v -run <NomeTest> ./internal/handler/...${RESET}"
  fi

  [ "$FAIL" -eq 0 ] && echo -e "\n  ${GREEN}${BOLD}TUTTI I TEST PASSATI${RESET}" \
                     || echo -e "\n  ${RED}${BOLD}BUILD FALLITA${RESET}"
  exit "$code"
}
trap on_exit EXIT

# ══════════════════════════════════════════════════════════════════════════════
# FASE 0 — prerequisiti
# ══════════════════════════════════════════════════════════════════════════════
section "Prerequisiti"
command -v go >/dev/null 2>&1 && ok "go $(go version | awk '{print $3}')" || { fail "go non trovato"; exit 1; }

if [ "$UNIT_ONLY" -eq 0 ]; then
  command -v docker >/dev/null 2>&1 && ok "docker" || { fail "docker non trovato (usa --unit-only per saltare)"; exit 1; }
fi

# ══════════════════════════════════════════════════════════════════════════════
# FASE A — unit test + stress test (niente Docker)
# ══════════════════════════════════════════════════════════════════════════════
if [ "$INTEGRATION_ONLY" -eq 0 ]; then

  section "A1 — build e vet"
  if go build -o /dev/null ./...; then ok "go build"; else fail "go build"; fi
  if go vet ./...; then ok "go vet"; else fail "go vet"; fi

  section "A2 — unit test + stress test (-race, tutti i pacchetti)"
  echo "  → go test $GT_FLAGS ./..."
  # shellcheck disable=SC2086
  if go test $GT_FLAGS ./...; then
    ok "unit + stress test"
  else
    fail "unit + stress test — vedi output sopra"
  fi

  section "A3 — stress test singoli pacchetti (report dettagliato)"
  # Lancia ogni pacchetto con stress test in modo isolato per output leggibile.
  STRESS_PKGS=(
    "github.com/marco-spagn/pcmi/internal/event"
    "github.com/marco-spagn/pcmi/internal/embedding"
    "github.com/marco-spagn/pcmi/internal/webhook"
    "github.com/marco-spagn/pcmi/internal/worker"
    "github.com/marco-spagn/pcmi/internal/service"
    "github.com/marco-spagn/pcmi/internal/handler"
  )
  for pkg in "${STRESS_PKGS[@]}"; do
    name="${pkg##*/}"
    # shellcheck disable=SC2086
    if go test $GT_FLAGS -run "Stress" "$pkg" 2>&1 | tail -3 | grep -qE "^ok|no test files"; then
      ok "stress ${name}"
    else
      # Rilancia senza grep per mostrare l'errore
      # shellcheck disable=SC2086
      go test $GT_FLAGS -run "Stress" "$pkg" || fail "stress ${name}"
    fi
  done

fi

if [ "$UNIT_ONLY" -eq 1 ]; then
  exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# FASE B — avvio Postgres (solo se non già attivo)
# ══════════════════════════════════════════════════════════════════════════════
section "B — infrastruttura (Postgres)"

pg_is_ready() {
  $DOCKER_COMPOSE exec -T postgres psql -U pcmi -d pcmi -c 'SELECT 1' >/dev/null 2>&1
}

if pg_is_ready; then
  ok "Postgres già attivo"
else
  echo "  → avvio postgres + redis via docker compose…"
  $DOCKER_COMPOSE up -d postgres redis >/dev/null
  POSTGRES_STARTED_BY_US=1

  echo "  → attendo che Postgres sia pronto (max 120s)…"
  OK=0
  for i in $(seq 1 120); do
    if pg_is_ready; then OK=1; break; fi
    sleep 1
    [ $((i % 10)) -eq 0 ] && echo "  … ${i}s"
  done
  if [ "$OK" -eq 1 ]; then
    ok "Postgres pronto"
  else
    fail "Postgres non risponde dopo 120s"
    exit 1
  fi
fi

export DATABASE_URL

# ══════════════════════════════════════════════════════════════════════════════
# FASE C — integration test standard
# ══════════════════════════════════════════════════════════════════════════════
section "C1 — integration test: handler (HTTP E2E)"
echo "  → DATABASE_URL=$DATABASE_URL"
# PCMI_SKIP_SSE_HTTPTEST=1: salta TestIntegrationHTTP_EventStreamMemoryStored
# che si blocca con adaptor+httptest senza un server TCP reale dedicato.
# Quel test è coperto da scripts/ci_integration_smoke.sh (curl su server reale).
# shellcheck disable=SC2086
if PCMI_SKIP_SSE_HTTPTEST=1 go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestIntegrationHTTP" \
    -timeout 10m; then
  ok "integration handler HTTP"
else
  fail "integration handler HTTP"
fi

section "C2 — integration test: gRPC (bufconn)"
GRPC_TEST_API_KEY="${GRPC_TEST_API_KEY:-testkey123}" \
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/grpc/... \
    -timeout 5m && ok "integration gRPC" || fail "integration gRPC"

# ══════════════════════════════════════════════════════════════════════════════
# FASE D — multi-instance integration test (il nuovo file)
# ══════════════════════════════════════════════════════════════════════════════
section "D1 — multi-instance: visibilità dati cross-replica"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_DataVisibilityAcrossInstances" \
    -timeout 2m -v && ok "DataVisibility" || fail "DataVisibility"

section "D2 — multi-instance: scritture concorrenti da tutte le repliche"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_ConcurrentWritesFromAllInstances" \
    -timeout 3m -v && ok "ConcurrentWrites (5 repliche × 100 goroutine)" || fail "ConcurrentWrites"

section "D3 — multi-instance: propagazione eventi Redis"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_RedisEventPropagation" \
    -timeout 2m -v && ok "RedisEventPropagation" || fail "RedisEventPropagation"

section "D4 — multi-instance: scritture concorrenti sullo stesso path"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_SamePathConcurrentWrites" \
    -timeout 2m -v && ok "SamePathConcurrentWrites" || fail "SamePathConcurrentWrites"

section "D5 — multi-instance: load misto lettura/scrittura (4 repliche)"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_MixedReadWriteLoad" \
    -timeout 5m -v && ok "MixedReadWriteLoad (4 × 100 goroutine)" || fail "MixedReadWriteLoad"

section "D6 — multi-instance: retrieve cross-replica"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_CrossInstanceRetrieve" \
    -timeout 2m -v && ok "CrossInstanceRetrieve" || fail "CrossInstanceRetrieve"

section "D7 — multi-instance: stats consistenti su tutte le repliche"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_StatsConsistencyAcrossInstances" \
    -timeout 2m -v && ok "StatsConsistency" || fail "StatsConsistency"

section "D8 — multi-instance: high-volume (6 repliche × 300 goroutine)"
echo "  ${YELLOW}⚠${RESET}  test pesante — budget 3 minuti"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_HighVolumeMultiReplica" \
    -timeout 4m -v && ok "HighVolume (1800 goroutine)" || fail "HighVolume"

# ══════════════════════════════════════════════════════════════════════════════
# FASE E — tutti i multi-instance insieme (run completo, una sola riga)
# ══════════════════════════════════════════════════════════════════════════════
section "E — multi-instance: suite completa (singolo comando)"
echo "  → run 'TestMultiInstance' — tutti i test in un colpo solo"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance" \
    -timeout 15m \
    && ok "suite multi-instance completa" || fail "suite multi-instance completa"
