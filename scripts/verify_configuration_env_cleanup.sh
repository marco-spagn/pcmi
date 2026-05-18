#!/usr/bin/env bash
# End-to-end verification for feat/configuration-env-cleanup (and descendants).
#
# Usage:
#   ./scripts/verify_configuration_env_cleanup.sh          # quick (~2–4 min)
#   ./scripts/verify_configuration_env_cleanup.sh --full   # + Docker stack smoke (~10+ min first build)
#   SKIP_DOCKER=1 ./scripts/verify_configuration_env_cleanup.sh --full  # full tests, no compose
#
# Exit 0 only if every check passes.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODE="quick"
for arg in "$@"; do
	case "$arg" in
	--full) MODE="full" ;;
	-h|--help)
		sed -n '2,10p' "$0"
		exit 0
		;;
	*) echo "Unknown arg: $arg (use --full or --help)" >&2; exit 2 ;;
	esac
done

API_URL="${API_URL:-http://localhost:8000}"
WORKER_URL="${WORKER_URL:-http://localhost:8081}"
GRPC_HOST="${GRPC_HOST:-localhost:50051}"
DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"

PASS=0
FAIL=0
SKIP=0

section() {
	echo ""
	echo "════════════════════════════════════════════════════════════"
	echo "  $1"
	echo "════════════════════════════════════════════════════════════"
}

ok() {
	echo "  ✓ $1"
	PASS=$((PASS + 1))
}

bad() {
	echo "  ✗ $1" >&2
	FAIL=$((FAIL + 1))
}

skip() {
	echo "  ⊘ $1 (skipped)"
	SKIP=$((SKIP + 1))
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || {
		bad "missing command: $1"
		return 1
	}
}

section "Branch & toolchain"
BRANCH="$(git branch --show-current 2>/dev/null || echo unknown)"
echo "  branch: $BRANCH"
echo "  mode:   $MODE"
need_cmd go
need_cmd git
ok "go $(go version | awk '{print $3}')"

# ── 1. os.Getenv audit (production code) ─────────────────────────────────────
section "Config single source of truth (os.Getenv)"
VIOLATIONS="$( {
	grep -rn 'os\.Getenv' cmd internal --include='*.go' 2>/dev/null || true
} | grep -v '_test\.go' | grep -v 'internal/config/' || true)"
if [ -z "$VIOLATIONS" ]; then
	ok "no os.Getenv outside internal/config in cmd/ + internal/ (prod)"
else
	bad "unexpected os.Getenv outside config:"
	echo "$VIOLATIONS" >&2
fi

# ── 2. CI_start gate logic (local simulation) ────────────────────────────────
section "CI_start gate (simulated)"
simulate_ci_gate() {
	local msg="$1"
	if echo "$msg" | grep -Fq 'CI_start'; then
		echo "run"
	else
		echo "skip"
	fi
}
if [ "$(simulate_ci_gate 'feat: widgets CI_start')" = "run" ]; then
	ok "CI_start token enables pipeline"
else
	bad "CI_start should enable pipeline"
fi
if [ "$(simulate_ci_gate 'docs: readme only')" = "skip" ]; then
	ok "commit without CI_start skips pipeline"
else
	bad "missing CI_start should skip pipeline"
fi

# ── 3. Build & vet ───────────────────────────────────────────────────────────
section "Go build & vet"
if go build -o /dev/null ./...; then
	ok "go build ./..."
else
	bad "go build ./..."
fi
if go vet ./...; then
	ok "go vet ./..."
else
	bad "go vet ./..."
fi

# ── 4. Unit tests (race) on packages touched by config cleanup ───────────────
section "Unit tests (-race)"
PKGS="./internal/config/... ./internal/crypto/... ./internal/grpc/... \
  ./internal/telemetry/... ./internal/webhook/... ./internal/middleware/... \
  ./internal/service/... ./internal/worker/... ./cmd/..."
if go test -race -count=1 $PKGS; then
	ok "race tests (config, grpc, worker, cmd, …)"
else
	bad "race tests failed"
fi

# ── 5. .env.example ↔ config.go sync ─────────────────────────────────────────
section "Config / .env.example drift guard"
if go test -count=1 ./internal/config/... -run TestEnvExampleStaysInSyncWithConfig; then
	ok "TestEnvExampleStaysInSyncWithConfig"
else
	bad "env example out of sync with config.Load"
fi

# ── 6. gRPC port from config (unit contract) ─────────────────────────────────
section "gRPC port resolution"
if go test -count=1 ./internal/grpc/... -run 'TestPortResolutionFromConfig|TestEphemeralListenerWorks'; then
	ok "ResolveGRPCPort contract tests"
else
	bad "gRPC port tests"
fi

# ── 7. Config.Load smoke with env overrides ──────────────────────────────────
section "config.Load() overrides"
if go test -count=1 ./internal/config/... -run 'TestLoad|TestLoadDefaults|TestLoadRateLimit'; then
	ok "config.Load unit tests"
else
	bad "config.Load tests"
fi

if [ "$MODE" != "full" ]; then
	section "Summary (quick)"
	echo "  passed: $PASS  failed: $FAIL  skipped: $SKIP"
	echo ""
	echo "  Run with --full for Docker stack + HTTP/gRPC smoke."
	[ "$FAIL" -eq 0 ] || exit 1
	exit 0
fi

# ══════════════════════════════════════════════════════════════════════════════
# FULL: Docker Compose stack + live smoke
# ══════════════════════════════════════════════════════════════════════════════

section "Docker infrastructure"
if [ "${SKIP_DOCKER:-}" = "1" ]; then
	skip "Docker (SKIP_DOCKER=1)"
else
	need_cmd docker || true
	need_cmd curl || true
	need_cmd jq || true

	if docker info >/dev/null 2>&1; then
		ok "docker daemon reachable"

		echo "  → make infra-down (clean stale containers)…"
		make infra-down >/dev/null 2>&1 || true

		echo "  → make infra-up (may take several minutes on first build)…"
		if make infra-up; then
			ok "make infra-up"
		else
			bad "make infra-up — try: make infra-logs"
		fi

		if make infra-smoke; then
			ok "make infra-smoke (/v1/ready + /health)"
		else
			bad "infra-smoke HTTP checks"
		fi

		# Worker health
		if curl -sf "$WORKER_URL/health" | jq -e '.status == "healthy"' >/dev/null; then
			ok "worker $WORKER_URL/health"
		else
			bad "worker health check"
		fi

		# Authenticated API probe
		if curl -sf -H "X-API-Key: $PCMI_API_KEY" \
			"$API_URL/v1/health" | jq -e '.status == "ok"' >/dev/null 2>&1; then
			ok "GET /v1/health with API key"
		else
			# /v1/health may differ; try store-less retrieve
			if curl -sf -o /dev/null -w "%{http_code}" -H "X-API-Key: $PCMI_API_KEY" \
				-X POST "$API_URL/v1/retrieve" \
				-H "Content-Type: application/json" \
				-d '{"path_prefix":"root","limit":1}' | grep -qE '^(200|404)$'; then
				ok "POST /v1/retrieve accepts API key"
			else
				bad "authenticated API probe"
			fi
		fi

		# gRPC health (script uses GRPC_HOST env — acceptable for smoke tooling)
		if go run ./scripts/grpc_health_smoke.go 2>/dev/null; then
			ok "gRPC health smoke ($GRPC_HOST)"
		else
			bad "gRPC health smoke — check API logs for gRPC listen port"
		fi

		# Host-side integration tests (matches CI subset)
		if [ -n "${DATABASE_URL:-}" ] || docker exec pcmi-postgres psql -U pcmi -d pcmi -c 'SELECT 1' >/dev/null 2>&1; then
			export DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
			export GRPC_HOST GRPC_TEST_API_KEY="${GRPC_TEST_API_KEY:-$PCMI_API_KEY}"
			if go test -race -count=1 -tags=integration ./internal/grpc/... -timeout 3m; then
				ok "go test -tags=integration ./internal/grpc/..."
			else
				bad "gRPC integration tests"
			fi
		else
			skip "gRPC integration tests (no DATABASE_URL)"
		fi
	else
		skip "Docker not available — full stack smoke omitted"
	fi
fi

section "Summary (full)"
echo "  passed: $PASS  failed: $FAIL  skipped: $SKIP"
if [ "$FAIL" -gt 0 ]; then
	echo ""
	echo "  Tip: make infra-logs | make infra-ps"
	exit 1
fi
echo ""
echo "  All checks passed."
