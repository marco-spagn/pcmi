#!/usr/bin/env bash
# Replica locale della pipeline GitHub Actions `.github/workflows/ci.yml`.
#
# Fasi (scripts/ci/phases/):
#   A — build/vet/config audit (job go)
#   B — golangci-lint
#   C — govulncheck
#   D — helm + kubeconform
#   E — Trivy (opz., RUN_TRIVY=1)
#   F — go test -race -tags=integration + coverage gate (job go)
#   G — integration-smoke (scripts/act_integration_smoke_host.sh)
#
# Non include: CodeQL, integration-e2e OpenAI (serve secret), push badge su main.
#
# Usage:
#   ./scripts/ci_like_github.sh                  # tutto (lungo)
#   ./scripts/ci_like_github.sh --integration-smoke
#   ./scripts/ci_like_github.sh --go-job
#
# Env:
#   SKIP_LINT=1 SKIP_GOVCHECK=1 SKIP_HELM=1 SKIP_COVERAGE=1
#   RUN_TRIVY=1  PCMI_GO_TEST_TIMEOUT  PCMI_GO_TEST_P=1
#   CI_LIKE_GO_VERBOSE=1  CI_LIKE_HEARTBEAT_SECS=120  CI_LIKE_NO_RACE=1
#   PCMI_SKIP_SSE_HTTPTEST — default 1 in Phase F (CI GitHub non lo imposta)
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
PHASES="$ROOT/scripts/ci/phases"

usage() {
  cat <<'EOF'
Uso: ./scripts/ci_like_github.sh [opzioni]

  (default) --full     Fasi A–G (parità CI GitHub, senza CodeQL/E2E OpenAI).

  --integration-smoke  Solo Phase G (make act-integration-smoke).

  --go-job             Fasi A–F senza integration-smoke.

Variabili: SKIP_LINT=1 SKIP_GOVCHECK=1 SKIP_HELM=1 SKIP_COVERAGE=1 RUN_TRIVY=1
           CI_LIKE_HEARTBEAT_SECS=120 CI_LIKE_GO_VERBOSE=1 CI_LIKE_NO_RACE=1
EOF
}

MODE=full
while [[ $# -gt 0 ]]; do
  case "$1" in
  --integration-smoke) MODE=smoke ;;
  --go-job) MODE=go ;;
  --full) MODE=full ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    echo "Opzione sconosciuta: $1" >&2
    usage >&2
    exit 2
    ;;
  esac
  shift
done

if [[ "$MODE" == "smoke" ]]; then
  exec bash "$ROOT/scripts/act_integration_smoke_host.sh"
fi

# shellcheck source=compose_postgres_wait.inc.sh
source "$ROOT/scripts/compose_postgres_wait.inc.sh"

say() {
  echo "[ci-like-github] $*"
}

run_phase() {
  local script="$1"
  chmod +x "$script"
  "$script"
}

say "Phase A — go build / vet / config audit (job CI: go)"
run_phase "$PHASES/a_go_static.sh"

say "Phase B — golangci-lint (job CI: golangci-lint)"
run_phase "$PHASES/b_lint.sh"

say "Phase C — govulncheck (job CI: security)"
run_phase "$PHASES/c_govulncheck.sh"

say "Phase D — helm lint + kubeconform (job CI: helm-lint)"
run_phase "$PHASES/d_helm.sh"

if [[ "${RUN_TRIVY:-}" == "1" ]]; then
  say "Phase E — Trivy scan immagini (job CI: trivy-images)"
  chmod +x scripts/ci/trivy_images.sh
  scripts/ci/trivy_images.sh
fi

if [[ "$MODE" == "full" || "$MODE" == "go" ]]; then
  say "Phase F — Postgres/Redis + go test -race -tags=integration (job CI: go)"
  docker info >/dev/null 2>&1 || {
    echo "[ci-like-github] Docker è necessario per la fase F" >&2
    exit 1
  }

  FREE_DEV_PORTS_LABEL="[ci-like-github]" bash "$ROOT/scripts/free_dev_ports.sh"
  $DOCKER_COMPOSE down -v --remove-orphans || true
  $DOCKER_COMPOSE up -d postgres redis

  COMPOSE_POSTGRES_WAIT_LABEL="[ci-like-github]"
  if ! compose_wait_postgres_ready 360; then
    echo "[ci-like-github] Postgres non risponde:" >&2
    $DOCKER_COMPOSE logs --tail=120 postgres || true
    exit 1
  fi

  export DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
  export PCMI_ENCRYPTION_KEY="${PCMI_ENCRYPTION_KEY:-01234567890123456789012345678901}"
  export PCMI_SKIP_SSE_HTTPTEST="${PCMI_SKIP_SSE_HTTPTEST:-1}"

  echo "[ci-like-github] go test ./internal/… — può restare muto molti minuti tra pacchetti (-race)."
  run_phase "$PHASES/f_go_integration.sh"
fi

if [[ "$MODE" == "full" ]]; then
  say "Phase G — integration-smoke (job CI: integration-smoke)"
  bash "$ROOT/scripts/act_integration_smoke_host.sh"
fi

say "Fine — controlli allineati alla CI superati."
