#!/usr/bin/env bash
# Replica locale della pipeline GitHub Actions `.github/workflows/ci.yml` quando
# i job sono abilitati (messaggio di commit con `CI_start` o workflow_dispatch).
#
# Esegue in sequenza (come sul runner, dove molti job sono paralleli):
#   - build / vet / test di audit config (stesso ordine logico del job `go`)
#   - golangci-lint, govulncheck, Helm + kubeconform (stessi job CI; skippabili)
#   - Postgres + Redis via docker compose, poi `go test -race -tags=integration ./internal/...`
#     con coverage + gate (job `go`)
#   - job `integration-smoke`: stesso flusso di `scripts/act_integration_smoke_host.sh`
#
# Non include: CodeQL, integration-e2e OpenAI (serve secret), push badge su main.
#
# Usage:
#   ./scripts/ci_like_github.sh                  # tutto (lungo)
#   ./scripts/ci_like_github.sh --integration-smoke   # solo smoke (~ come dopo il merge dei job)
#   ./scripts/ci_like_github.sh --go-job         # solo test Go + DB + coverage gate
#
# Env:
#   SKIP_LINT=1 SKIP_GOVCHECK=1 SKIP_HELM=1 SKIP_COVERAGE=1  — salta le fasi indicate
#   RUN_TRIVY=1  — anche scan immagini Docker (`make act-trivy`, lento)
#   PCMI_GO_TEST_TIMEOUT — timeout globale go test (default 45m)
#   PCMI_GO_TEST_P=1 — serializza i pacchetti (-race + RAM limitata su laptop)
#   CI_LIKE_GO_VERBOSE=1 — passa -v ai test (vedi avanzamento)
#   CI_LIKE_HEARTBEAT_SECS — se >0, messaggio ogni N secondi mentre gira go test (es. 120)
#   PCMI_SKIP_SSE_HTTPTEST — default 1 in Phase F: salta TestIntegrationHTTP_EventStreamMemoryStored
#                            (httptest+SSE instabile con -race); Phase G copre SSE via curl.
#                            Imposta 0 per forzare quel test in locale.
#   CI_LIKE_NO_RACE=1 — Phase F senza -race (molto più veloce; la CI GitHub usa ancora -race)
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"

usage() {
  cat <<'EOF'
Uso: ./scripts/ci_like_github.sh [opzioni]

  (default) --full     Build/vet/audit, lint opz., govulncheck, helm, test Go+Postgres,
                       coverage gate, poi integration-smoke (stesso stack degli script CI).

  --integration-smoke  Solo job integration-smoke (compose + binari host + bash/go/SDK).
                       Equivalente a: make act-integration-smoke

  --go-job             Solo fasi statiche + Postgres + go test -race -tags=integration
                       ./internal/... + gate coverage (senza integration-smoke).

Variabili: SKIP_LINT=1 SKIP_GOVCHECK=1 SKIP_HELM=1 SKIP_COVERAGE=1 RUN_TRIVY=1
           CI_LIKE_HEARTBEAT_SECS=120 CI_LIKE_GO_VERBOSE=1 CI_LIKE_NO_RACE=1 PCMI_SKIP_SSE_HTTPTEST=0

Fase F (`go test ./internal/...`, di default con `-race`): può restare muta molti minuti tra un pacchetto e
l'altro (normale). Dopo internal/grpc restano handler → repository → service → worker (i più lenti).
Su CI lo step è spesso ~5–20 min; su Mac con -race e PCMI_GO_TEST_P=1 è realistico 15–45+ min.
PCMI_GO_TEST_TIMEOUT (default 45m). Progressione: CI_LIKE_GO_VERBOSE=1 oppure CI_LIKE_HEARTBEAT_SECS=120.
Velocità: PCMI_SKIP_SSE_HTTPTEST=1 (default nello script), oppure CI_LIKE_NO_RACE=1 per Phase F senza race.

Su GitHub la pipeline completa parte solo con CI_start nel messaggio di commit; questo
script non applica quel gate. integration-e2e (OpenAI) non è incluso.
EOF
}

MODE=full
while [[ $# -gt 0 ]]; do
  case "$1" in
  --integration-smoke)
    MODE=smoke
    ;;
  --go-job)
    MODE=go
    ;;
  --full)
    MODE=full
    ;;
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

say "Phase A — go build / vet / config audit (job CI: go)"
go build ./...
go vet ./...
go test -count=1 ./internal/config/... \
  -run 'TestNoOSGetenvOutsideConfig|TestEnvExampleStaysInSyncWithConfig'

if [[ "${SKIP_LINT:-}" != "1" ]]; then
  say "Phase B — golangci-lint (job CI: golangci-lint)"
  GOLANGCI_IMAGE="${GOLANGCI_IMAGE:-golangci/golangci-lint:v2.12.2}"
  if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint run ./...
  elif docker info >/dev/null 2>&1; then
    docker run --rm -v "$ROOT:/app" -w /app "$GOLANGCI_IMAGE" golangci-lint run ./...
  else
    echo "[ci-like-github] SKIP golangci-lint — installa golangci-lint oppure avvia Docker" >&2
  fi
else
  say "SKIP golangci-lint (SKIP_LINT=1)"
fi

if [[ "${SKIP_GOVCHECK:-}" != "1" ]]; then
  say "Phase C — govulncheck (job CI: security)"
  go run golang.org/x/vuln/cmd/govulncheck@latest ./...
else
  say "SKIP govulncheck (SKIP_GOVCHECK=1)"
fi

if [[ "${SKIP_HELM:-}" != "1" ]]; then
  say "Phase D — helm lint + kubeconform (job CI: helm-lint)"
  HELM_IMAGE="${HELM_IMAGE:-alpine/helm:3.14.4}"
  KUBECONFORM_IMAGE="${KUBECONFORM_IMAGE:-ghcr.io/yannh/kubeconform:v0.6.7}"
  RENDERED="/tmp/pcmi-ci-like-rendered-$$-$RANDOM.yaml"
  run_helm_ci() {
    if command -v helm >/dev/null 2>&1; then
      helm "$@"
    else
      docker run --rm -v "$ROOT:/work" -w /work "$HELM_IMAGE" "$@"
    fi
  }
  run_helm_ci lint deploy/helm/pcmi --strict
  run_helm_ci template pcmi deploy/helm/pcmi >"$RENDERED"
  if [[ ! -s "$RENDERED" ]]; then
    echo "[ci-like-github] helm template produced empty file" >&2
    exit 1
  fi
  if command -v kubeconform >/dev/null 2>&1; then
    kubeconform -summary -strict -schema-location default "$RENDERED"
  elif docker info >/dev/null 2>&1; then
    docker run --rm -v /tmp:/tmp:ro "$KUBECONFORM_IMAGE" -summary -strict -schema-location default "$RENDERED"
  else
    echo "[ci-like-github] SKIP kubeconform — installa kubeconform o avvia Docker" >&2
  fi
  rm -f "$RENDERED"
else
  say "SKIP Helm/kubeconform (SKIP_HELM=1)"
fi

if [[ "${RUN_TRIVY:-}" == "1" ]]; then
  say "Phase E — Trivy scan immagini (job CI: trivy-images)"
  make act-trivy
fi

if [[ "$MODE" == "full" || "$MODE" == "go" ]]; then
  say "Phase F — Postgres/Redis puliti + go test -race -tags=integration ./internal/... (job CI: go)"
  docker info >/dev/null 2>&1 || {
    echo "[ci-like-github] Docker è necessario per la fase F" >&2
    exit 1
  }

  FRESH=1
  $DOCKER_COMPOSE down -v --remove-orphans || true
  $DOCKER_COMPOSE up -d postgres redis

  COMPOSE_POSTGRES_WAIT_LABEL="[ci-like-github]"
  if ! compose_wait_postgres_ready 360; then
    echo "[ci-like-github] Postgres non risponde — ultimi log:" >&2
    $DOCKER_COMPOSE logs --tail=120 postgres || true
    exit 1
  fi

  export DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
  export PCMI_ENCRYPTION_KEY="${PCMI_ENCRYPTION_KEY:-01234567890123456789012345678901}"

  # Stesso motivo dello skip su GITHUB_ACTIONS nel test: httptest+SSE+race si blocca spesso in locale.
  export PCMI_SKIP_SSE_HTTPTEST="${PCMI_SKIP_SSE_HTTPTEST:-1}"

  GT_TIMEOUT="${PCMI_GO_TEST_TIMEOUT:-45m}"
  # Un solo array (mai vuoto): con `set -u`, su Bash 3.2 (macOS) "${arr[@]}" su array vuoto fallisce.
  GT_ARGS=( -count=1 -tags=integration )
  if [[ "${CI_LIKE_NO_RACE:-}" != "1" ]]; then
    GT_ARGS=( -race "${GT_ARGS[@]}" )
  fi
  if [[ -n "${PCMI_GO_TEST_P:-}" ]]; then
    GT_ARGS+=( -p "${PCMI_GO_TEST_P}" )
  fi
  GT_ARGS+=( -timeout "$GT_TIMEOUT" )
  if [[ "${CI_LIKE_GO_VERBOSE:-}" == "1" ]]; then
    GT_ARGS+=( -v )
  fi
  GT_ARGS+=( -coverprofile=coverage.out -covermode=atomic )

  echo "[ci-like-github] Avvio go test -race -tags=integration ./internal/… — tra un pacchetto e l'altro può non comparire nulla per molti minuti (normale)."
  echo "[ci-like-github] Timeout globale: ${GT_TIMEOUT}${PCMI_GO_TEST_P:+ — parallelismo pacchetti: -p ${PCMI_GO_TEST_P}}${CI_LIKE_GO_VERBOSE:+ — verbose}${CI_LIKE_HEARTBEAT_SECS:+ — heartbeat ogni ${CI_LIKE_HEARTBEAT_SECS}s}"
  echo "[ci-like-github] Indicativo: CI runner ~5–20 min per questo step; Mac -race -p1 spesso 15–45+ min."
  echo "[ci-like-github] Velocità: PCMI_SKIP_SSE_HTTPTEST=${PCMI_SKIP_SSE_HTTPTEST} (0 per eseguire SSE httptest); CI_LIKE_NO_RACE=${CI_LIKE_NO_RACE:-0}"

  _heartbeat_pid=
  if [[ "${CI_LIKE_HEARTBEAT_SECS:-0}" =~ ^[1-9][0-9]*$ ]]; then
    (
      while sleep "${CI_LIKE_HEARTBEAT_SECS}"; do
        ts="$(date +%H:%M:%S)"
        lines="$(pgrep -fl '[g]o test' 2>/dev/null | head -5 || true)"
        if [[ -n "$lines" ]]; then
          echo "[ci-like-github] heartbeat ${ts} — processi go test / compilazione:" >&2
          echo "$lines" >&2
        else
          echo "[ci-like-github] heartbeat ${ts} — nessun 'go test' in elenco (compile/link o gap tra pacchetti)" >&2
        fi
      done
    ) &
    _heartbeat_pid=$!
  fi

  _go_ec=0
  go test "${GT_ARGS[@]}" ./internal/... || _go_ec=$?

  if [[ -n "${_heartbeat_pid:-}" ]]; then
    kill "${_heartbeat_pid}" 2>/dev/null || true
    wait "${_heartbeat_pid}" 2>/dev/null || true
  fi

  [[ $_go_ec -eq 0 ]] || exit "$_go_ec"

  if [[ "${SKIP_COVERAGE:-}" != "1" ]]; then
    chmod +x scripts/ci_coverage_check.sh
    COVERAGE_MIN_TOTAL="${COVERAGE_MIN_TOTAL:-39}" \
      COVERAGE_PKG_FLOORS="${COVERAGE_PKG_FLOORS:-config:70,event:70,eventschema:85,repository:70,service:70,worker:19}" \
      COVERAGE_OMIT_PROTOBUF=true \
      bash scripts/ci_coverage_check.sh
  else
    say "SKIP coverage gate (SKIP_COVERAGE=1)"
  fi
fi

if [[ "$MODE" == "full" ]]; then
  say "Phase G — integration-smoke (job CI: integration-smoke → act_integration_smoke_host.sh)"
  bash "$ROOT/scripts/act_integration_smoke_host.sh"
fi

say "Fine — controlli allineati alla CI superati."
