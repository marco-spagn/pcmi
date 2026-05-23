#!/usr/bin/env bash
# Simulazione locale production-like: parità CI host + E2E OpenAI (opz.) + MCP + feature smokes.
#
# Usage:
#   make test-full-real
#   ./scripts/run_full_validation.sh
#
# Env (pass-through to ci_like_github.sh):
#   CI_LIKE_NO_RACE=1          — Phase F senza -race (~2× più veloce; GitHub usa ancora -race)
#   PCMI_GO_TEST_P=1           — serializza pacchetti go test (laptop)
#   CI_LIKE_HEARTBEAT_SECS=120 — heartbeat durante go test lunghi
#   SKIP_LINT=1 SKIP_GOVCHECK=1 SKIP_HELM=1 SKIP_COVERAGE=1 — salta fasi CI
#
# E2E OpenAI (Phase 2 — come job GitHub integration-e2e):
#   Richiede OPENAI_API_KEY in ambiente o in .env (non vuota).
#   FULL_VALIDATION_E2E=distill — invece del trio script, esegue:
#     make distillation-e2e PRESET=soc SYNTH_NUM=100
#   Default FULL_VALIDATION_E2E=trio:
#     scripts/e2e/test_pcmi.sh, ci_e2e_sse_dedup.sh, ci_e2e_finale.sh
#
# Infra (Phase 3 — un solo avvio stack per smoke HTTP/MCP):
#   SKIP_INFRA_DOWN=1 — non esegue make infra-down a fine validazione (debug locale)
#
# Prerequisites: go, docker, curl, jq, git; psql on PATH per ci-like-github smoke;
#   python3 per distillation-e2e; OPENAI opzionale per Phase 2.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

API_URL="${API_URL:-http://localhost:8000}"
DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
FULL_VALIDATION_E2E="${FULL_VALIDATION_E2E:-trio}"
INFRA_STARTED=0

say() { echo "[test-full-real] $*"; }
warn() { echo "[test-full-real] WARN: $*" >&2; }

openai_configured() {
  if [ -n "${OPENAI_API_KEY:-}" ]; then
    return 0
  fi
  if [ -f .env ] && grep -qE '^OPENAI_API_KEY=.+$' .env 2>/dev/null; then
    return 0
  fi
  return 1
}

sync_openai_to_env_file() {
  if [ -n "${OPENAI_API_KEY:-}" ] && [ -f .env ]; then
    if grep -q '^OPENAI_API_KEY=' .env 2>/dev/null; then
      # shellcheck disable=SC2016
      sed -i.bak "s|^OPENAI_API_KEY=.*|OPENAI_API_KEY=${OPENAI_API_KEY}|" .env
    else
      echo "OPENAI_API_KEY=${OPENAI_API_KEY}" >> .env
    fi
    rm -f .env.bak
  fi
}

api_ready() {
  curl -sf "${API_URL}/v1/ready" 2>/dev/null \
    | jq -e '.status == "ready" and .database_ok == true and .redis_ok == true' >/dev/null 2>&1
}

ensure_api_stack() {
  if api_ready; then
    say "API pronta su ${API_URL} (/v1/ready)"
    return 0
  fi
  say "Avvio stack Docker (make infra-up) per smoke HTTP/MCP…"
  make infra-up
  INFRA_STARTED=1
  if ! api_ready; then
    warn "API non risponde su /v1/ready dopo infra-up"
    exit 1
  fi
}

maybe_infra_down() {
  if [[ "${SKIP_INFRA_DOWN:-}" == "1" ]]; then
    warn "SKIP_INFRA_DOWN=1 — stack Docker lasciato attivo"
    return 0
  fi
  if [[ "$INFRA_STARTED" == "1" ]]; then
    say "Arresto stack Docker (make infra-down)…"
    make infra-down
  fi
}

usage() {
  sed -n '2,30p' "$0"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

say "Phase 0 — free-dev-ports (act-preflight)"
make act-preflight

make env >/dev/null 2>&1 || cp .env.example .env

say "Phase 1 — parità CI GitHub (ci_like_github.sh) — può richiedere 20–45+ min"
chmod +x scripts/ci_like_github.sh scripts/free_dev_ports.sh
bash scripts/ci_like_github.sh

say "Phase 2 — integration-e2e OpenAI (job CI integration-e2e)"
if openai_configured; then
  sync_openai_to_env_file
  case "$FULL_VALIDATION_E2E" in
  distill | distillation)
    say "E2E distillation (PRESET=soc SYNTH_NUM=100)"
    make distillation-e2e PRESET=soc SYNTH_NUM=100
    ;;
  trio | *)
    chmod +x scripts/e2e/test_pcmi.sh scripts/ci_e2e_sse_dedup.sh scripts/ci_e2e_finale.sh
    say "E2E trio: test_pcmi.sh → ci_e2e_sse_dedup → ci_e2e_finale"
    ./scripts/e2e/test_pcmi.sh
    ./scripts/ci_e2e_sse_dedup.sh
    ./scripts/ci_e2e_finale.sh
    ;;
  esac
else
  warn "SKIP Phase 2 — OPENAI_API_KEY non impostata (export OPENAI_API_KEY=… o valorizza .env)"
  warn "         Per il job GitHub integration-e2e: trio script o FULL_VALIDATION_E2E=distill"
fi

say "Phase 3 — feature smokes + MCP (stack API unico)"
ensure_api_stack
export API_URL PCMI_BASE_URL="${API_URL}" PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"

chmod +x scripts/smoke_importance_retrieve.sh scripts/smoke_sessions.sh
say "  smoke-importance (PCMI-009)"
SKIP_READY=1 make smoke-importance
say "  smoke-sessions (PCMI-010 curl E2E)"
SKIP_READY=1 make smoke-sessions
say "  MCP build + unit + smoke JSON-RPC"
make build-mcp
make test-mcp-unit
make test-mcp-smoke

say "Phase 4 — agent sessions integration (PCMI-010 go test)"
export DATABASE_URL
PCMI_SKIP_SSE_HTTPTEST=1 make test-sessions-integration

maybe_infra_down

say "Fine — validazione completa terminata."
