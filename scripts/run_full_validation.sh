#!/usr/bin/env bash
# Local production-like simulation: host CI parity + E2E OpenAI (opt.) + MCP + feature smokes.
#
# Usage:
#   make test-full-real
#   ./scripts/run_full_validation.sh
#
# Env (pass-through to ci_like_github.sh):
#   CI_LIKE_NO_RACE=1          — Phase F without -race (~2× faster; GitHub still uses -race)
#   PCMI_GO_TEST_P=1           — serialize go test packages (laptop)
#   CI_LIKE_HEARTBEAT_SECS=120 — heartbeat during long go tests
#   SKIP_LINT=1 SKIP_GOVCHECK=1 SKIP_HELM=1 SKIP_COVERAGE=1 — skip CI phases
#
# E2E OpenAI (Phase 2 — like GitHub job integration-e2e):
#   Requires OPENAI_API_KEY in environment or in .env (non-empty).
#   FULL_VALIDATION_E2E=distill — instead of the script trio, runs:
#     make distillation-e2e PRESET=soc SYNTH_NUM=100
#   Default FULL_VALIDATION_E2E=trio:
#     scripts/e2e/test_pcmi.sh, ci_e2e_sse_dedup.sh, ci_e2e_finale.sh
#
# Infra (Phase 3 — single stack startup for HTTP/MCP smoke):
#   SKIP_INFRA_DOWN=1 — do not run make infra-down at end of validation (local debug)
#
# Prerequisites: go, docker, curl, jq, git; psql on PATH for ci-like-github smoke;
#   python3 for distillation-e2e; OPENAI optional for Phase 2.
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
    say "API ready on ${API_URL} (/v1/ready)"
    return 0
  fi
  say "Starting Docker stack (make infra-up) for HTTP/MCP smoke..."
  make infra-up
  INFRA_STARTED=1
  if ! api_ready; then
    warn "API not responding on /v1/ready after infra-up"
    exit 1
  fi
}

maybe_infra_down() {
  if [[ "${SKIP_INFRA_DOWN:-}" == "1" ]]; then
    warn "SKIP_INFRA_DOWN=1 — Docker stack left running"
    return 0
  fi
  if [[ "$INFRA_STARTED" == "1" ]]; then
    say "Stopping Docker stack (make infra-down)..."
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

say "Phase 1 — GitHub CI parity (ci_like_github.sh) — may take 20–45+ min"
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
  warn "SKIP Phase 2 — OPENAI_API_KEY not set (export OPENAI_API_KEY=... or set it in .env)"
  warn "         For GitHub integration-e2e job: script trio or FULL_VALIDATION_E2E=distill"
fi

say "Phase 3 — feature smokes + MCP (stack API unico)"
ensure_api_stack
export API_URL PCMI_BASE_URL="${API_URL}" PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"

chmod +x scripts/smoke_importance_retrieve.sh scripts/smoke_sessions.sh scripts/smoke_dedup.sh
say "  smoke-importance (PCMI-009)"
SKIP_READY=1 make smoke-importance
say "  smoke-sessions (PCMI-010 curl E2E)"
SKIP_READY=1 make smoke-sessions
say "  smoke-dedup (PCMI-011 curl E2E)"
SKIP_READY=1 make smoke-dedup
say "  MCP build + unit + smoke JSON-RPC"
make build-mcp
make test-mcp-unit
make test-mcp-smoke

say "Phase 4 — agent sessions integration (PCMI-010 go test)"
export DATABASE_URL
PCMI_SKIP_SSE_HTTPTEST=1 make test-sessions-integration

maybe_infra_down

say "Done — full validation complete."
