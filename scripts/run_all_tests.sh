#!/usr/bin/env bash
# run_all_tests.sh — runs unit tests, stress tests, and integration tests in sequence.
#
# Usage:
#   ./scripts/run_all_tests.sh                  # everything (starts Postgres via Docker)
#   ./scripts/run_all_tests.sh --unit-only       # unit + stress only (no Docker)
#   ./scripts/run_all_tests.sh --integration-only # integration only (Postgres already running)
#   ./scripts/run_all_tests.sh --no-cleanup      # leave Postgres running after
#   PCMI_GO_TEST_P=2 ./scripts/run_all_tests.sh  # limit package parallelism
#
# Colored output. Exit code 1 if at least one phase fails.
#
# Prerequisites: go, docker (for integration). curl/jq optional.

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
    *) echo "Unknown argument: $arg" >&2; exit 2 ;;
  esac
done

# ── colori ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'

PASS=0; FAIL=0; SKIP=0
FAILED_ITEMS=()         # collects label + section of each failure
CURRENT_SECTION="?"     # updated by section()

section() {
  CURRENT_SECTION="$1"
  echo -e "\n${CYAN}${BOLD}══ $1 ══${RESET}"
}
ok()   { echo -e "  ${GREEN}${RESET} $1"; PASS=$((PASS+1)); }
fail() {
  echo -e "  ${RED}${RESET} $1" >&2
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
    echo -e "\n${YELLOW}→ stopping Postgres started by this script...${RESET}"
    $DOCKER_COMPOSE stop postgres redis >/dev/null 2>&1 || true
  fi
  echo -e "\n${BOLD}══ SUMMARY ══${RESET}"
  echo -e "  ${GREEN} passed${RESET}:   $PASS"
  echo -e "  ${RED} failed${RESET}:   $FAIL"
  echo -e "  ${YELLOW}⊘ skipped${RESET}:  $SKIP"

  if [ "${#FAILED_ITEMS[@]}" -gt 0 ]; then
    echo -e "\n${RED}${BOLD}╔══ WHAT FAILED ══╗${RESET}"
    for item in "${FAILED_ITEMS[@]}"; do
      echo -e "  ${RED}${RESET}  $item"
    done
    echo -e "${RED}${BOLD}╚════════════════════╝${RESET}"
    echo -e "\n  Tip: re-run only the failures with:"
    echo -e "  ${CYAN}DATABASE_URL=... go test -race -count=1 -tags integration -v -run <TestName> ./internal/handler/...${RESET}"
  fi

  [ "$FAIL" -eq 0 ] && echo -e "\n  ${GREEN}${BOLD}ALL TESTS PASSED${RESET}" \
                     || echo -e "\n  ${RED}${BOLD}BUILD FAILED${RESET}"
  exit "$code"
}
trap on_exit EXIT

# ══════════════════════════════════════════════════════════════════════════════
# FASE 0 — prerequisiti
# ══════════════════════════════════════════════════════════════════════════════
section "Prerequisiti"
command -v go >/dev/null 2>&1 && ok "go $(go version | awk '{print $3}')" || { fail "go not found"; exit 1; }

if [ "$UNIT_ONLY" -eq 0 ]; then
  command -v docker >/dev/null 2>&1 && ok "docker" || { fail "docker not found (use --unit-only to skip)"; exit 1; }
fi

# ══════════════════════════════════════════════════════════════════════════════
# FASE A — unit test + stress test (niente Docker)
# ══════════════════════════════════════════════════════════════════════════════
if [ "$INTEGRATION_ONLY" -eq 0 ]; then

  section "A1 — build e vet"
  if go build -o /dev/null ./...; then ok "go build"; else fail "go build"; fi
  if go vet ./...; then ok "go vet"; else fail "go vet"; fi

  section "A2 — unit test + stress test (-race, all packages)"
  echo "  → go test $GT_FLAGS ./..."
  # shellcheck disable=SC2086
  if go test $GT_FLAGS ./...; then
    ok "unit + stress test"
  else
    fail "unit + stress test — see output above"
  fi

  section "A3 — stress test individual packages (detailed report)"
  # Runs each package with stress tests in isolation for readable output.
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
      # Re-run without grep to show the error
      # shellcheck disable=SC2086
      go test $GT_FLAGS -run "Stress" "$pkg" || fail "stress ${name}"
    fi
  done

fi

if [ "$UNIT_ONLY" -eq 1 ]; then
  exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# PHASE B — start Postgres (only if not already running)
# ══════════════════════════════════════════════════════════════════════════════
section "B — infrastructure (Postgres)"

pg_is_ready() {
  $DOCKER_COMPOSE exec -T postgres psql -U pcmi -d pcmi -c 'SELECT 1' >/dev/null 2>&1
}

if pg_is_ready; then
  ok "Postgres already running"
else
  echo "  → starting postgres + redis via docker compose..."
  $DOCKER_COMPOSE up -d postgres redis >/dev/null
  POSTGRES_STARTED_BY_US=1

  echo "  → waiting for Postgres to be ready (max 120s)..."
  OK=0
  for i in $(seq 1 120); do
    if pg_is_ready; then OK=1; break; fi
    sleep 1
    [ $((i % 10)) -eq 0 ] && echo "  … ${i}s"
  done
  if [ "$OK" -eq 1 ]; then
    ok "Postgres ready"
  else
    fail "Postgres not responding after 120s"
    exit 1
  fi
fi

export DATABASE_URL

# ══════════════════════════════════════════════════════════════════════════════
# FASE C — integration test standard
# ══════════════════════════════════════════════════════════════════════════════
section "C1 — integration test: handler (HTTP E2E)"
echo "  → DATABASE_URL=$DATABASE_URL"
# PCMI_SKIP_SSE_HTTPTEST=1: skip TestIntegrationHTTP_EventStreamMemoryStored
# which blocks with adaptor+httptest without a dedicated real TCP server.
# That test is covered by scripts/ci_integration_smoke.sh (curl on real server).
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
section "D1 — multi-instance: data visibility across replicas"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_DataVisibilityAcrossInstances" \
    -timeout 2m -v && ok "DataVisibility" || fail "DataVisibility"

section "D2 — multi-instance: concurrent writes from all replicas"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_ConcurrentWritesFromAllInstances" \
    -timeout 3m -v && ok "ConcurrentWrites (5 replicas × 100 goroutines)" || fail "ConcurrentWrites"

section "D3 — multi-instance: Redis event propagation"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_RedisEventPropagation" \
    -timeout 2m -v && ok "RedisEventPropagation" || fail "RedisEventPropagation"

section "D4 — multi-instance: concurrent writes to the same path"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_SamePathConcurrentWrites" \
    -timeout 2m -v && ok "SamePathConcurrentWrites" || fail "SamePathConcurrentWrites"

section "D5 — multi-instance: mixed read/write load (4 replicas)"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_MixedReadWriteLoad" \
    -timeout 5m -v && ok "MixedReadWriteLoad (4 × 100 goroutines)" || fail "MixedReadWriteLoad"

section "D6 — multi-instance: retrieve cross-replica"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_CrossInstanceRetrieve" \
    -timeout 2m -v && ok "CrossInstanceRetrieve" || fail "CrossInstanceRetrieve"

section "D7 — multi-instance: consistent stats across all replicas"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_StatsConsistencyAcrossInstances" \
    -timeout 2m -v && ok "StatsConsistency" || fail "StatsConsistency"

section "D8 — multi-instance: high-volume (6 replicas × 300 goroutines)"
echo "  ${YELLOW}${RESET}  heavy test — 3 minute budget"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance_HighVolumeMultiReplica" \
    -timeout 4m -v && ok "HighVolume (1800 goroutines)" || fail "HighVolume"

# ══════════════════════════════════════════════════════════════════════════════
# PHASE E — all multi-instance tests together (full run, single command)
# ══════════════════════════════════════════════════════════════════════════════
section "E — multi-instance: full suite (single command)"
echo "  → run 'TestMultiInstance' — all tests at once"
# shellcheck disable=SC2086
go test $GT_FLAGS -tags integration \
    ./internal/handler/... \
    -run "TestMultiInstance" \
    -timeout 15m \
    && ok "multi-instance suite complete" || fail "multi-instance suite complete"
