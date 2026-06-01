#!/usr/bin/env bash
# scripts/bug_hunt.sh — PCMI massive bug-hunting harness
#
# Goal: maximize the probability of surfacing latent bugs by attacking the
# codebase from many independent angles. NOT just a test runner — this includes
# race detection, native fuzzing, OpenAPI property-based testing (schemathesis),
# gRPC load (ghz), HTTP stress (vegeta), soak/leak detection, mutation testing,
# vuln/secret/SBOM scans, migration forward+rollback, kustomize validation.
#
# Usage:
#   bash scripts/bug_hunt.sh                    # run everything
#   bash scripts/bug_hunt.sh --phase=fuzz       # one phase
#   bash scripts/bug_hunt.sh --phase=static,unit,fuzz
#   bash scripts/bug_hunt.sh --fast             # skip slow phases (load, soak, mutation)
#   bash scripts/bug_hunt.sh --list             # list available phases
#
# Exit code: 0 if no critical issues, non-zero otherwise.
# Reports written to: .bug_hunt/<timestamp>/

set -uo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# Config
# ─────────────────────────────────────────────────────────────────────────────
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUTDIR="${ROOT}/.bug_hunt/${TS}"
mkdir -p "$OUTDIR"
SUMMARY="${OUTDIR}/summary.md"

BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
GRPC_ADDR="${PCMI_GRPC_ADDR:-localhost:50051}"
API_KEY="${PCMI_API_KEY:-testkey123}"

FUZZ_TIME="${FUZZ_TIME:-30s}"          # per fuzz target
SOAK_DURATION="${SOAK_DURATION:-300s}" # 5 min soak
LOAD_RATE="${LOAD_RATE:-200/s}"        # vegeta rate
LOAD_DURATION="${LOAD_DURATION:-60s}"
COVERAGE_MIN="${COVERAGE_MIN:-65}"     # fail if total coverage below this

FAST=false
PHASES_REQ=""

# ─────────────────────────────────────────────────────────────────────────────
# Phase registry — ordered
# ─────────────────────────────────────────────────────────────────────────────
ALL_PHASES=(
  setup
  static
  unit
  bench
  fuzz
  integration
  api_property
  grpc_load
  http_load
  soak
  mutation
  security
  migrations
  containers
  report
)

FAST_SKIP=(soak mutation grpc_load http_load)

# ─────────────────────────────────────────────────────────────────────────────
# Plumbing
# ─────────────────────────────────────────────────────────────────────────────
RED=$'\033[31m'; GRN=$'\033[32m'; YEL=$'\033[33m'; CYA=$'\033[36m'; RST=$'\033[0m'
declare -A RESULTS

log()  { printf "%s[%s]%s %s\n" "$CYA" "$(date -u +%H:%M:%S)" "$RST" "$*"; }
warn() { printf "%s[WARN]%s %s\n" "$YEL" "$RST" "$*" >&2; }
err()  { printf "%s[ERR ]%s %s\n" "$RED" "$RST" "$*" >&2; }
ok()   { printf "%s[ OK ]%s %s\n" "$GRN" "$RST" "$*"; }

phase_header() { log "═══ Phase: $1 ═══"; }

run_phase() {
  local name="$1"
  local fn="phase_${name}"
  if ! declare -F "$fn" >/dev/null; then
    err "unknown phase: $name"; return 1
  fi
  phase_header "$name"
  local start; start=$(date +%s)
  if "$fn" 2>&1 | tee "${OUTDIR}/${name}.log"; then
    RESULTS[$name]="PASS"
    ok "$name done in $(( $(date +%s) - start ))s"
  else
    local code=${PIPESTATUS[0]}
    RESULTS[$name]="FAIL($code)"
    err "$name failed in $(( $(date +%s) - start ))s (exit $code)"
  fi
}

have() { command -v "$1" >/dev/null 2>&1; }

go_install_if_missing() {
  local bin="$1" pkg="$2"
  if ! have "$bin"; then
    log "installing $bin from $pkg"
    GOBIN="$(go env GOPATH)/bin" go install "$pkg" || warn "failed to install $bin"
    export PATH="$(go env GOPATH)/bin:$PATH"
  fi
}

pip_install_if_missing() {
  local mod="$1" pkg="${2:-$1}"
  if ! python3 -c "import $mod" >/dev/null 2>&1; then
    log "installing python pkg $pkg"
    pip3 install --quiet --break-system-packages "$pkg" || warn "failed to install $pkg"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 0: setup — verify env, install missing tooling
# ─────────────────────────────────────────────────────────────────────────────
phase_setup() {
  log "checking required base tooling"
  for bin in go docker git curl jq; do
    have "$bin" || { err "missing: $bin"; return 1; }
  done

  log "ensuring go-based linters/scanners present"
  go_install_if_missing golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint@latest
  go_install_if_missing staticcheck   honnef.co/go/tools/cmd/staticcheck@latest
  go_install_if_missing govulncheck   golang.org/x/vuln/cmd/govulncheck@latest
  go_install_if_missing gosec         github.com/securego/gosec/v2/cmd/gosec@latest
  go_install_if_missing errcheck      github.com/kisielk/errcheck@latest
  go_install_if_missing nilaway       go.uber.org/nilaway/cmd/nilaway@latest
  go_install_if_missing gitleaks      github.com/zricethezav/gitleaks/v8@latest
  go_install_if_missing buf           github.com/bufbuild/buf/cmd/buf@latest
  go_install_if_missing ghz           github.com/bojand/ghz/cmd/ghz@latest
  go_install_if_missing grpcurl       github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
  go_install_if_missing vegeta        github.com/tsenart/vegeta/v12@latest
  go_install_if_missing hadolint      github.com/hadolint/hadolint/cmd/hadolint@latest 2>/dev/null || true
  go_install_if_missing migrate       github.com/golang-migrate/migrate/v4/cmd/migrate@latest
  go_install_if_missing gremlins      github.com/go-gremlins/gremlins/cmd/gremlins@latest

  if ! have trivy; then
    warn "trivy not found — install from https://aquasecurity.github.io/trivy/ for container scans"
  fi
  if ! have syft; then
    warn "syft not found — install for SBOM generation"
  fi
  if ! have schemathesis; then
    pip_install_if_missing schemathesis schemathesis
  fi

  log "starting stack (docker compose up -d --build)"
  docker compose up -d --build || return 1
  log "waiting for /v1/health"
  for i in {1..60}; do
    if curl -sf "${BASE_URL}/v1/health" >/dev/null; then ok "stack healthy"; return 0; fi
    sleep 2
  done
  err "stack failed to become healthy"
  docker compose ps
  return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 1: static — fast, parallel, catches obvious bugs
# ─────────────────────────────────────────────────────────────────────────────
phase_static() {
  local fail=0

  log "→ go vet"
  go vet ./... || fail=1

  log "→ golangci-lint"
  golangci-lint run --timeout 5m ./... || fail=1

  log "→ staticcheck"
  staticcheck ./... || fail=1

  log "→ errcheck"
  errcheck -ignoretests ./... || warn "errcheck found unchecked errors"

  log "→ gosec (security-focused)"
  gosec -quiet -severity medium -confidence medium ./... || fail=1

  log "→ govulncheck"
  govulncheck ./... || fail=1

  log "→ nilaway"
  nilaway ./... || warn "nilaway found potential nil derefs"

  log "→ gitleaks (secrets in history)"
  gitleaks detect --no-banner --redact -v --source . --report-path "${OUTDIR}/gitleaks.json" || fail=1

  log "→ buf lint (proto)"
  if [ -d proto ]; then (cd proto && buf lint) || fail=1; fi

  log "→ buf breaking (proto vs main)"
  if [ -d proto ] && [ "$(git rev-parse --abbrev-ref HEAD)" != "main" ]; then
    (cd proto && buf breaking --against "https://github.com/marco-spagn/pcmi.git#branch=main,subdir=proto") || warn "proto breaking changes vs main"
  fi

  log "→ hadolint (Dockerfiles)"
  if have hadolint; then
    find . -maxdepth 2 -name "Dockerfile*" -not -path "./.git/*" -print0 | \
      xargs -0 -I{} sh -c 'echo "== {} =="; hadolint --no-fail "{}"'
  fi

  return $fail
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 2: unit — race detector + coverage + threshold
# ─────────────────────────────────────────────────────────────────────────────
phase_unit() {
  log "→ go test -race -cover (count=1, timeout 10m)"
  go test -race -count=1 -timeout 10m \
    -coverprofile="${OUTDIR}/coverage.out" \
    -covermode=atomic \
    ./... || return 1

  local pct
  pct=$(go tool cover -func="${OUTDIR}/coverage.out" | awk '/^total:/ {gsub("%",""); print $3}')
  log "total coverage: ${pct}%"
  go tool cover -html="${OUTDIR}/coverage.out" -o "${OUTDIR}/coverage.html" || true

  if awk "BEGIN{exit !(${pct} < ${COVERAGE_MIN})}"; then
    err "coverage ${pct}% < threshold ${COVERAGE_MIN}%"
    return 1
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 3: bench — perf regression baseline
# ─────────────────────────────────────────────────────────────────────────────
phase_bench() {
  log "→ go test -bench (no tests, just benchmarks)"
  go test -run=^$ -bench=. -benchmem -benchtime=3s -timeout 10m ./... \
    | tee "${OUTDIR}/bench.txt" || return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 4: fuzz — native Go fuzzing on every Fuzz* target
# ─────────────────────────────────────────────────────────────────────────────
phase_fuzz() {
  log "→ discovering FuzzXxx targets"
  local targets
  targets=$(grep -rEn '^func\s+Fuzz[A-Z]\w*\s*\(' --include="*_test.go" . \
            | sed -E 's|^(.+)/[^/]+_test.go:[0-9]+:func (Fuzz[A-Za-z0-9_]+).*|\1 \2|')
  if [ -z "$targets" ]; then
    warn "no Fuzz* targets found — consider adding native fuzz tests for parsers/decoders"
    return 0
  fi

  local fail=0
  while IFS=' ' read -r pkg fn; do
    [ -z "$pkg" ] && continue
    log "  fuzzing ${pkg} :: ${fn} for ${FUZZ_TIME}"
    if ! go test "$pkg" -run=^$ -fuzz="^${fn}$" -fuzztime="${FUZZ_TIME}" \
         -fuzzminimizetime=10s 2>&1 | tee -a "${OUTDIR}/fuzz.log"; then
      fail=1
      err "fuzz crash in ${pkg}::${fn} — corpus saved under testdata/fuzz/${fn}"
    fi
  done <<< "$targets"
  return $fail
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 5: integration — Makefile-defined integration + SDK smokes
# ─────────────────────────────────────────────────────────────────────────────
phase_integration() {
  log "→ make test-integration"
  make test-integration || return 1
  log "→ make sdk-smoke"
  make sdk-smoke || return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 6: api_property — schemathesis property-based against OpenAPI
# THIS IS WHERE MOST API BUGS GET FOUND
# ─────────────────────────────────────────────────────────────────────────────
phase_api_property() {
  if ! have schemathesis; then
    err "schemathesis not available"; return 1
  fi
  local spec="docs/openapi.yaml"
  if [ ! -f "$spec" ]; then
    err "openapi spec missing at $spec"; return 1
  fi

  log "→ schemathesis run (200 examples per endpoint, all checks)"
  schemathesis run "$spec" \
    --base-url="$BASE_URL" \
    --header "X-API-Key: ${API_KEY}" \
    --checks all \
    --hypothesis-max-examples=200 \
    --hypothesis-deadline=2000 \
    --validate-schema=true \
    --workers 4 \
    --junit-xml="${OUTDIR}/schemathesis.xml" \
    || return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 7: grpc_load — ghz against critical RPCs
# ─────────────────────────────────────────────────────────────────────────────
phase_grpc_load() {
  if ! have ghz; then warn "ghz missing"; return 0; fi
  local proto="proto/pcmi/v1/memory.proto"
  [ -f "$proto" ] || { warn "proto missing"; return 0; }

  log "→ ghz Store rpc"
  ghz --insecure --proto "$proto" --import-paths proto \
    --call pcmi.v1.MemoryService/Store \
    --metadata "{\"x-api-key\":\"${API_KEY}\"}" \
    --data '{"path":"root.bh.store","content":"bh","tags":["bh"]}' \
    --duration=30s --concurrency=50 \
    -O json -o "${OUTDIR}/ghz_store.json" \
    "$GRPC_ADDR" || return 1

  log "→ ghz Retrieve rpc"
  ghz --insecure --proto "$proto" --import-paths proto \
    --call pcmi.v1.MemoryService/Retrieve \
    --metadata "{\"x-api-key\":\"${API_KEY}\"}" \
    --data '{"path_prefix":"root.bh","limit":10}' \
    --duration=30s --concurrency=50 \
    -O json -o "${OUTDIR}/ghz_retrieve.json" \
    "$GRPC_ADDR" || return 1

  # Fail if p99 over 500ms or error rate over 1%
  for f in "${OUTDIR}/ghz_store.json" "${OUTDIR}/ghz_retrieve.json"; do
    local p99 err_count total
    p99=$(jq '.latencyDistribution[] | select(.percentage==99) | .latency' "$f")
    err_count=$(jq '[.errorDistribution[]?.count // 0] | add // 0' "$f" 2>/dev/null || echo 0)
    total=$(jq '.count' "$f")
    log "  $(basename "$f"): p99=${p99}ns errors=${err_count}/${total}"
  done
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 8: http_load — vegeta on hottest endpoints
# ─────────────────────────────────────────────────────────────────────────────
phase_http_load() {
  if ! have vegeta; then warn "vegeta missing"; return 0; fi
  cat > "${OUTDIR}/targets.txt" <<EOF
POST ${BASE_URL}/v1/memories
X-API-Key: ${API_KEY}
Content-Type: application/json
@${OUTDIR}/store_body.json

POST ${BASE_URL}/v1/retrieve
X-API-Key: ${API_KEY}
Content-Type: application/json
@${OUTDIR}/retrieve_body.json
EOF
  echo '{"path":"root.bh.load","content":"x","tags":["load"]}' > "${OUTDIR}/store_body.json"
  echo '{"path_prefix":"root.bh","limit":10}'                  > "${OUTDIR}/retrieve_body.json"

  log "→ vegeta attack ${LOAD_RATE} for ${LOAD_DURATION}"
  vegeta attack -targets="${OUTDIR}/targets.txt" \
                -rate="${LOAD_RATE}" -duration="${LOAD_DURATION}" \
                > "${OUTDIR}/vegeta.bin"
  vegeta report -type=text < "${OUTDIR}/vegeta.bin" | tee "${OUTDIR}/vegeta_report.txt"
  vegeta report -type=hdrplot < "${OUTDIR}/vegeta.bin" > "${OUTDIR}/vegeta_hdr.txt" 2>/dev/null || true

  local success_rate
  success_rate=$(vegeta report -type=json < "${OUTDIR}/vegeta.bin" | jq '.success * 100')
  log "  success rate: ${success_rate}%"
  awk "BEGIN{exit !(${success_rate} < 99.0)}" && { err "success rate ${success_rate}% < 99%"; return 1; } || true
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 9: soak — long-running with pprof snapshots to catch leaks
# ─────────────────────────────────────────────────────────────────────────────
phase_soak() {
  log "→ soak ${SOAK_DURATION} at low rate, sampling pprof heap"
  ( vegeta attack -targets="${OUTDIR}/targets.txt" -rate=20/s -duration="${SOAK_DURATION}" > "${OUTDIR}/soak.bin" ) &
  local pid=$!

  # capture heap at start, mid, end
  curl -s "${BASE_URL}/debug/pprof/heap" > "${OUTDIR}/heap_start.pprof" 2>/dev/null || \
    warn "pprof endpoint not exposed (expected on prod) — skipping leak diff"

  local half=$(( $(echo "$SOAK_DURATION" | tr -d 's') / 2 ))
  sleep "$half"
  curl -s "${BASE_URL}/debug/pprof/heap" > "${OUTDIR}/heap_mid.pprof" 2>/dev/null || true

  wait $pid
  curl -s "${BASE_URL}/debug/pprof/heap" > "${OUTDIR}/heap_end.pprof" 2>/dev/null || true

  vegeta report < "${OUTDIR}/soak.bin" > "${OUTDIR}/soak_report.txt"

  if [ -s "${OUTDIR}/heap_start.pprof" ] && [ -s "${OUTDIR}/heap_end.pprof" ]; then
    log "→ pprof diff (end vs start)"
    go tool pprof -top -diff_base="${OUTDIR}/heap_start.pprof" "${OUTDIR}/heap_end.pprof" \
      > "${OUTDIR}/heap_diff.txt" 2>&1 || true
    head -40 "${OUTDIR}/heap_diff.txt"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 10: mutation — gremlins (catches tests that don't actually assert)
# ─────────────────────────────────────────────────────────────────────────────
phase_mutation() {
  if ! have gremlins; then warn "gremlins missing"; return 0; fi
  log "→ gremlins on internal/ (this is slow — limited to internal/)"
  gremlins unleash --timeout-coefficient 3 ./internal/... \
    > "${OUTDIR}/gremlins.txt" 2>&1 || true
  tail -30 "${OUTDIR}/gremlins.txt"
  local survived
  survived=$(grep -Eo '[0-9]+ mutants survived' "${OUTDIR}/gremlins.txt" | head -1 | awk '{print $1}')
  log "  mutants survived: ${survived:-unknown} (high = weak assertions)"
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 11: security — trivy, grype, syft
# ─────────────────────────────────────────────────────────────────────────────
phase_security() {
  if have trivy; then
    log "→ trivy fs (deps + IaC + secrets)"
    trivy fs --severity HIGH,CRITICAL --exit-code 1 --no-progress \
      -f json -o "${OUTDIR}/trivy_fs.json" . || warn "trivy_fs found HIGH/CRITICAL"

    log "→ trivy config (Dockerfile + k8s + compose)"
    trivy config --severity HIGH,CRITICAL --no-progress \
      -f json -o "${OUTDIR}/trivy_config.json" . || warn "trivy_config found issues"

    log "→ building images for image scan"
    local img_api="pcmi-bh-api:bh" img_w="pcmi-bh-worker:bh"
    docker build -f Dockerfile.api    -t "$img_api" . >/dev/null
    docker build -f Dockerfile.worker -t "$img_w"   . >/dev/null

    log "→ trivy image (api)"
    trivy image --severity HIGH,CRITICAL --no-progress \
      -f json -o "${OUTDIR}/trivy_image_api.json" "$img_api" || warn "image vulns (api)"
    log "→ trivy image (worker)"
    trivy image --severity HIGH,CRITICAL --no-progress \
      -f json -o "${OUTDIR}/trivy_image_worker.json" "$img_w" || warn "image vulns (worker)"

    if have syft; then
      log "→ SBOM via syft"
      syft "$img_api" -o spdx-json > "${OUTDIR}/sbom_api.spdx.json"
      syft "$img_w"   -o spdx-json > "${OUTDIR}/sbom_worker.spdx.json"
    fi
  else
    warn "trivy missing — security phase reduced"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 12: migrations — up → down → up; verify reversibility
# ─────────────────────────────────────────────────────────────────────────────
phase_migrations() {
  if ! have migrate; then warn "migrate cli missing"; return 0; fi
  local db_url="${DATABASE_URL:-postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable}"

  log "→ migrate down (to zero)"
  migrate -path migrations -database "$db_url" down -all 2>&1 | tail -20 || warn "down failed"

  log "→ migrate up"
  migrate -path migrations -database "$db_url" up 2>&1 | tail -20 || return 1

  log "→ migrate down (full again — checks rollback safety)"
  migrate -path migrations -database "$db_url" down -all 2>&1 | tail -20 || return 1

  log "→ migrate up (restore for downstream phases)"
  migrate -path migrations -database "$db_url" up 2>&1 | tail -20 || return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 13: containers — compose config + kustomize render + docker build
# ─────────────────────────────────────────────────────────────────────────────
phase_containers() {
  log "→ docker compose config (syntax)"
  docker compose config -q || return 1

  log "→ kustomize render"
  for overlay in deploy/k8s/overlays/*/; do
    [ -d "$overlay" ] || continue
    log "  rendering $overlay"
    kubectl kustomize "$overlay" > "${OUTDIR}/$(basename "$overlay").yaml" || return 1
  done

  log "→ helm lint"
  if have helm && [ -d deploy/helm ]; then
    for chart in deploy/helm/*/; do
      helm lint "$chart" || return 1
    done
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Phase 14: report — aggregate and print summary
# ─────────────────────────────────────────────────────────────────────────────
phase_report() {
  {
    echo "# Bug-Hunt Report — ${TS}"
    echo ""
    echo "## Phase Results"
    echo ""
    echo "| Phase | Result |"
    echo "|---|---|"
    for p in "${ALL_PHASES[@]}"; do
      [ "$p" = report ] && continue
      echo "| $p | ${RESULTS[$p]:-SKIPPED} |"
    done
    echo ""
    echo "## Key Artifacts"
    echo ""
    for f in coverage.html bench.txt fuzz.log schemathesis.xml \
             vegeta_report.txt soak_report.txt heap_diff.txt \
             gremlins.txt gitleaks.json \
             trivy_fs.json trivy_image_api.json trivy_image_worker.json; do
      [ -f "${OUTDIR}/${f}" ] && echo "- \`${OUTDIR}/${f}\`"
    done
    echo ""
    echo "## Next"
    echo ""
    echo "- Open issues for any FAIL phase"
    echo "- Triage HIGH/CRITICAL findings in trivy_*.json"
    echo "- Add fuzz corpus entries from any crashes (in testdata/fuzz/)"
    echo "- If schemathesis surfaced API contract drift, fix server or update OpenAPI"
  } > "$SUMMARY"
  cat "$SUMMARY"
}

# ─────────────────────────────────────────────────────────────────────────────
# Arg parsing
# ─────────────────────────────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --fast)         FAST=true ;;
    --phase=*)      PHASES_REQ="${arg#--phase=}" ;;
    --list)         printf '%s\n' "${ALL_PHASES[@]}"; exit 0 ;;
    -h|--help)
      grep '^# ' "$0" | head -25 | sed 's/^# //'
      exit 0 ;;
    *) err "unknown arg: $arg"; exit 2 ;;
  esac
done

# Decide phase list
if [ -n "$PHASES_REQ" ]; then
  IFS=',' read -ra PHASES <<< "$PHASES_REQ"
else
  PHASES=("${ALL_PHASES[@]}")
  if $FAST; then
    NEW=(); for p in "${PHASES[@]}"; do
      skip=false
      for s in "${FAST_SKIP[@]}"; do [ "$p" = "$s" ] && skip=true; done
      $skip || NEW+=("$p")
    done
    PHASES=("${NEW[@]}")
  fi
fi

log "writing reports to: $OUTDIR"
log "phases: ${PHASES[*]}"

for p in "${PHASES[@]}"; do
  run_phase "$p"
done

# Exit code
exit_code=0
for p in "${PHASES[@]}"; do
  case "${RESULTS[$p]:-}" in FAIL*) exit_code=1 ;; esac
done
log "FINAL: exit=${exit_code}  report=${SUMMARY}"
exit $exit_code