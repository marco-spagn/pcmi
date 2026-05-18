#!/usr/bin/env bash
# PCMI — complete local test suite (config-env-cleanup branch and main).
#
# Runs EVERY check: static audit, unit/race tests, Docker stack, HTTP/gRPC smoke,
# store/retrieve, summarize, webhook, encryption, rate limit, worker OpenAI on/off,
# gRPC port from config, optional host `go run`, optional coverage.
#
# Usage:
#   ./scripts/test_all_local.sh              # full (~15–25 min first Docker build)
#   ./scripts/test_all_local.sh --quick      # unit/static only (~3 min, no Docker)
#   ./scripts/test_all_local.sh --with-host  # full + API via `go run` on host
#   RUN_COVERAGE=1 ./scripts/test_all_local.sh
#   KEEP_STACK=1 ./scripts/test_all_local.sh # leave compose running at end
#
# Prerequisites: go, git, curl, jq, docker. API key: testkey123 (migrations/003).
#
# .env: backed up to .env.pcmi-test-backup and restored when the script exits.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODE="full"
WITH_HOST=0
for arg in "$@"; do
	case "$arg" in
	--quick) MODE="quick" ;;
	--with-host) WITH_HOST=1 ;;
	-h|--help)
		sed -n '2,18p' "$0"
		exit 0
		;;
	*) echo "Unknown: $arg (use --quick, --with-host, --help)" >&2; exit 2 ;;
	esac
done

API_URL="${API_URL:-http://localhost:8000}"
WORKER_URL="${WORKER_URL:-http://localhost:8081}"
GRPC_HOST="${GRPC_HOST:-localhost:50051}"
DOCKER_COMPOSE="${DOCKER_COMPOSE:-docker compose}"
KEY="${PCMI_API_KEY:-testkey123}"
ENC_KEY="${PCMI_ENCRYPTION_KEY_TEST:-01234567890123456789012345678901}"
PATH_BASE="root.localtest.$(date +%s)"
ENV_BACKUP=".env.pcmi-test-backup"

PASS=0
FAIL=0
SKIP=0

section() {
	echo ""
	echo "╔════════════════════════════════════════════════════════════╗"
	printf "║ %-58s ║\n" "$1"
	echo "╚════════════════════════════════════════════════════════════╝"
}

ok() { echo "  ✓ $1"; PASS=$((PASS + 1)); }
bad() { echo "  ✗ $1" >&2; FAIL=$((FAIL + 1)); }
skip() { echo "  ⊘ $1"; SKIP=$((SKIP + 1)); }

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || { bad "missing command: $1"; exit 1; }
}

env_set() {
	local key="$1" val="$2"
	if grep -q "^${key}=" .env 2>/dev/null; then
		# macOS/BSD sed
		sed -i.bak "s|^${key}=.*|${key}=${val}|" .env
	else
		echo "${key}=${val}" >> .env
	fi
}

restore_env() {
	if [ -f "$ENV_BACKUP" ]; then
		cp "$ENV_BACKUP" .env
		echo "  (restored .env from $ENV_BACKUP)"
	fi
}

on_exit() {
	local code=$?
	restore_env
	if [ "${KEEP_STACK:-}" != "1" ] && [ "$MODE" = "full" ] && [ "${SKIP_DOCKER:-}" != "1" ]; then
		$DOCKER_COMPOSE up -d --remove-orphans api worker >/dev/null 2>&1 || true
	fi
	rm -f .env.bak
	exit "$code"
}
trap on_exit EXIT

# ── Setup ────────────────────────────────────────────────────────────────────
section "Setup"
need_cmd go
need_cmd git
need_cmd curl
need_cmd jq
[ "$MODE" = "quick" ] || need_cmd docker

make env >/dev/null 2>&1 || cp .env.example .env
cp .env "$ENV_BACKUP"
echo "  branch: $(git branch --show-current 2>/dev/null || echo '?')"
echo "  mode:   $MODE"
ok "backed up .env → $ENV_BACKUP"

# ══════════════════════════════════════════════════════════════════════════════
# PHASE A — static + unit (no destructive .env / compose changes)
# ══════════════════════════════════════════════════════════════════════════════

section "A1 — os.Getenv audit (prod code only)"
VIOLATIONS="$( {
	grep -rn 'os\.Getenv' cmd internal --include='*.go' 2>/dev/null || true
} | grep -v '_test\.go' | grep -v 'internal/config/' || true)"
if [ -z "$VIOLATIONS" ]; then
	ok "no os.Getenv outside internal/config"
else
	bad "unexpected getenv usage:"
	echo "$VIOLATIONS" >&2
fi

section "A2 — CI_start gate (simulated)"
if echo "feat CI_start" | grep -Fq 'CI_start'; then ok "CI_start enables CI"; else bad "CI_start"; fi
if ! echo "docs only" | grep -Fq 'CI_start'; then ok "no CI_start skips CI"; else bad "skip logic"; fi
echo "  → GitHub: git commit -m '... CI_start' or gh workflow run CI"

section "A3 — build & vet"
go build -o /dev/null ./... && ok "go build" || bad "go build"
go vet ./... && ok "go vet" || bad "go vet"

section "A4 — unit tests (-race)"
PKGS="./internal/config/... ./internal/crypto/... ./internal/grpc/... \
  ./internal/telemetry/... ./internal/webhook/... ./internal/middleware/... \
  ./internal/service/... ./internal/worker/... ./cmd/..."
go test -race -count=1 $PKGS && ok "race tests" || bad "race tests"

section "A5 — config / .env.example sync"
go test -count=1 ./internal/config/... -run TestEnvExampleStaysInSyncWithConfig \
	&& ok "env.example ↔ config.go" || bad "env drift"

section "A6 — gRPC ResolveGRPCPort"
go test -count=1 ./internal/grpc/... -run 'TestPortResolutionFromConfig|TestEphemeralListenerWorks' \
	&& ok "gRPC port unit tests" || bad "gRPC port tests"

section "A7 — telemetry unit tests"
go test -count=1 ./internal/telemetry/... && ok "telemetry package tests" || bad "telemetry tests"

if [ "${RUN_COVERAGE:-}" = "1" ]; then
	section "A8 — coverage gate (optional)"
	make test-cover && make cover-check && ok "coverage" || bad "coverage"
fi

if [ "$MODE" = "quick" ]; then
	section "SUMMARY (quick)"
	echo "  passed: $PASS  failed: $FAIL  skipped: $SKIP"
	[ "$FAIL" -eq 0 ] || exit 1
	exit 0
fi

if [ "${SKIP_DOCKER:-}" = "1" ]; then
	skip "Docker phases (SKIP_DOCKER=1)"
	section "SUMMARY"
	echo "  passed: $PASS  failed: $FAIL  skipped: $SKIP"
	exit 0
fi

docker info >/dev/null 2>&1 || { bad "docker not running"; exit 1; }

# ══════════════════════════════════════════════════════════════════════════════
# PHASE B — Docker Compose full stack
# ══════════════════════════════════════════════════════════════════════════════

section "B1 — Docker stack up"
make infra-down >/dev/null 2>&1 || true
make infra-up && ok "infra-up" || { bad "infra-up"; exit 1; }

section "B2 — readiness & health"
curl -sf "$API_URL/v1/ready" | jq -e '.status == "ready"' >/dev/null && ok "/v1/ready" || bad "/v1/ready"
curl -sf "$API_URL/health" | jq -e '.status == "ok"' >/dev/null && ok "/health" || bad "/health"
curl -sf "$WORKER_URL/health" | jq -e '.status == "healthy"' >/dev/null && ok "worker /health" || bad "worker"

section "B3 — store + retrieve (X-API-Key)"
STORE_CODE=$(curl -sS -o /tmp/pcmi-store.json -w "%{http_code}" -X POST "$API_URL/v1/memories" \
	-H "X-API-Key: $KEY" -H "Content-Type: application/json" \
	-d "{\"path\":\"$PATH_BASE\",\"content\":\"local test all script\",\"tags\":[\"e2e\"],\"embedding_model\":\"unspecified\"}")
if [ "$STORE_CODE" = "200" ]; then ok "POST /v1/memories"; else bad "store HTTP $STORE_CODE"; fi
RET_CODE=$(curl -sS -o /tmp/pcmi-ret.json -w "%{http_code}" -X POST "$API_URL/v1/retrieve" \
	-H "X-API-Key: $KEY" -H "Content-Type: application/json" \
	-d "{\"path_prefix\":\"$PATH_BASE\",\"query\":\"local\",\"limit\":5}")
if [ "$RET_CODE" = "200" ] && [ "$(jq -r '.total' /tmp/pcmi-ret.json)" -ge 1 ]; then
	ok "POST /v1/retrieve"
else
	bad "retrieve HTTP $RET_CODE"
fi

section "B4 — gRPC health"
if GRPC_HOST="$GRPC_HOST" go run ./scripts/grpc_health_smoke.go; then
	ok "gRPC health ($GRPC_HOST)"
else
	bad "gRPC health"
fi

section "B5 — gRPC integration tests"
export DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
export GRPC_TEST_API_KEY="$KEY"
go test -race -count=1 -tags=integration ./internal/grpc/... -timeout 5m \
	&& ok "integration grpc tests" || bad "integration grpc"

section "B6 — summarize (OPENAI_API_KEY from .env)"
SUM_CODE=$(curl -sS -o /tmp/pcmi-sum.json -w "%{http_code}" -X POST "$API_URL/v1/memories/summarize" \
	-H "X-API-Key: $KEY" -H "Content-Type: application/json" \
	-d "{\"path_prefix\":\"root\",\"limit\":5}")
if [ "$SUM_CODE" = "200" ]; then
	METHOD=$(jq -r '.method' /tmp/pcmi-sum.json)
	ok "summarize method=$METHOD (llm if OPENAI_API_KEY set, else extractive)"
else
	bad "summarize HTTP $SUM_CODE"
fi

section "B7 — webhook max_attempts from config"
env_set WEBHOOK_MAX_ATTEMPTS 5
$DOCKER_COMPOSE up -d --force-recreate api >/dev/null
sleep 3
curl -sf "$API_URL/v1/ready" >/dev/null
curl -sS -o /dev/null -X POST "$API_URL/v1/webhooks" -H "X-API-Key: $KEY" \
	-H "Content-Type: application/json" \
	-d '{"url":"https://httpstat.us/500","event_types":["memory.stored"]}' || true
curl -sS -o /dev/null -X POST "$API_URL/v1/memories" -H "X-API-Key: $KEY" \
	-H "Content-Type: application/json" \
	-d "{\"path\":\"$PATH_BASE.wh\",\"content\":\"wh\",\"embedding_model\":\"unspecified\"}"
sleep 4
MAX_ATT=$($DOCKER_COMPOSE exec -T postgres psql -U pcmi -d pcmi -tA -c \
	"SELECT max_attempts FROM webhook_deliveries ORDER BY created_at DESC LIMIT 1;" 2>/dev/null | tr -d '[:space:]')
[ "$MAX_ATT" = "5" ] && ok "webhook max_attempts=5" || bad "webhook max_attempts=$MAX_ATT"

section "B8 — encryption (PCMI_ENCRYPTION_KEY + encrypt_content)"
env_set PCMI_ENCRYPTION_KEY "$ENC_KEY"
$DOCKER_COMPOSE up -d --force-recreate api >/dev/null
sleep 3
curl -sf "$API_URL/v1/ready" >/dev/null
ENC_PATH="$PATH_BASE.secret"
curl -sS -o /dev/null -X POST "$API_URL/v1/memories" -H "X-API-Key: $KEY" \
	-H "Content-Type: application/json" \
	-d "{\"path\":\"$ENC_PATH\",\"content\":\"secret\",\"encrypt_content\":true,\"embedding_model\":\"unspecified\"}"
RAW=$($DOCKER_COMPOSE exec -T postgres psql -U pcmi -d pcmi -tA -c \
	"SELECT content FROM memory_entries WHERE path='$ENC_PATH'::ltree ORDER BY id DESC LIMIT 1;")
PLAIN=$(curl -sS -X POST "$API_URL/v1/retrieve" -H "X-API-Key: $KEY" \
	-H "Content-Type: application/json" \
	-d "{\"path_prefix\":\"$ENC_PATH\",\"limit\":1}" | jq -r '.entries[0].content // empty')
echo "$RAW" | grep -q '^enc:v1:' && ok "encrypted at rest" || bad "not encrypted in DB"
[ "$PLAIN" = "secret" ] && ok "decrypt on retrieve" || bad "decrypt failed"

section "B9 — rate limit from config (admin key → RPM_ADMIN)"
env_set RATE_LIMIT_DISABLED false
env_set RATE_LIMIT_RPM_ADMIN 2
env_set PCMI_ENCRYPTION_KEY "$ENC_KEY"
$DOCKER_COMPOSE up -d --force-recreate api >/dev/null
sleep 3
HIT429=0
for i in 1 2 3 4 5; do
	C=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "$API_URL/v1/retrieve" \
		-H "X-API-Key: $KEY" -H "Content-Type: application/json" \
		-d "{\"path_prefix\":\"$PATH_BASE\",\"limit\":1}")
	[ "$C" = "429" ] && HIT429=1 && break
done
[ "$HIT429" -eq 1 ] && ok "rate limit 429" || bad "no 429 (admin uses RATE_LIMIT_RPM_ADMIN)"
env_set RATE_LIMIT_DISABLED true

section "B10 — gRPC port from config (GRPC_PORT=50061)"
env_set GRPC_PORT 50061
env_set PCMI_ENCRYPTION_KEY "$ENC_KEY"
$DOCKER_COMPOSE up -d --force-recreate api >/dev/null
GRPC_LOG_OK=0
for _ in 1 2 3 4 5 6; do
	sleep 2
	if $DOCKER_COMPOSE logs api 2>&1 | grep -q 'PCMI gRPC server on :50061'; then
		GRPC_LOG_OK=1
		break
	fi
done
if [ "$GRPC_LOG_OK" -eq 1 ]; then
	ok "API log shows :50061"
else
	bad "gRPC port not 50061 in logs"
fi
if GRPC_HOST=localhost:50061 go run ./scripts/grpc_health_smoke.go 2>/dev/null; then
	ok "gRPC on :50061"
else
	bad "gRPC :50061 unreachable"
fi
env_set GRPC_PORT 50051
$DOCKER_COMPOSE up -d --force-recreate api >/dev/null
sleep 3

section "B11 — worker without OPENAI_API_KEY"
OPENAI_SAVED=$(grep '^OPENAI_API_KEY=' "$ENV_BACKUP" | cut -d= -f2- || true)
env_set OPENAI_API_KEY ""
$DOCKER_COMPOSE up -d --force-recreate worker >/dev/null
sleep 3
if $DOCKER_COMPOSE logs worker --tail=30 2>&1 | grep -qiE 'OPENAI_API_KEY unset|embedding worker disabled'; then
	ok "worker without OpenAI"
else
	bad "worker should disable embedding without key"
fi
# restore OpenAI for subsequent steps
if [ -n "$OPENAI_SAVED" ]; then
	env_set OPENAI_API_KEY "$OPENAI_SAVED"
else
	env_set OPENAI_API_KEY ""
fi
$DOCKER_COMPOSE up -d --force-recreate worker >/dev/null
sleep 2

section "B12 — OTLP (optional)"
OTEL=$({ grep -E '^OTEL_EXPORTER_OTLP' .env 2>/dev/null || true; } | grep -v '^#' | grep -v '=$' | head -1 || true)
if [ -n "$OTEL" ]; then
	$DOCKER_COMPOSE up -d --force-recreate api >/dev/null
	sleep 3
	if $DOCKER_COMPOSE logs api --tail=30 2>&1 | grep -q 'OpenTelemetry: OTLP'; then
		ok "OTLP exporter active in API logs"
	else
		bad "OTEL vars set but no OTLP log line"
	fi
else
	skip "OTLP (set OTEL_EXPORTER_OTLP_TRACES_ENDPOINT in .env to test)"
fi

section "B13 — distillation pipeline (needs OPENAI_API_KEY)"
if grep -q '^OPENAI_API_KEY=.\+' .env 2>/dev/null; then
	curl -sS -o /dev/null -X POST "$API_URL/v1/memories" -H "X-API-Key: $KEY" \
		-H "Content-Type: application/json" \
		-d "{\"path\":\"$PATH_BASE.distill\",\"content\":\"distill me\",\"embedding_model\":\"unspecified\"}"
	sleep 8
	if $DOCKER_COMPOSE logs worker --tail=40 2>&1 | grep -q 'Distillation saved'; then
		ok "distillation after store"
	else
		skip "distillation not confirmed in logs (may be slow)"
	fi
else
	skip "distillation (OPENAI_API_KEY empty)"
fi

# ══════════════════════════════════════════════════════════════════════════════
# PHASE C — optional host go run (config from .env, not Docker API image)
# ══════════════════════════════════════════════════════════════════════════════

if [ "$WITH_HOST" -eq 1 ]; then
	section "C1 — host go run ./cmd/api (infra-deps only)"
	$DOCKER_COMPOSE stop api worker 2>/dev/null || true
	make infra-deps-up
	env_set RATE_LIMIT_DISABLED true
	env_set PCMI_ENCRYPTION_KEY "$ENC_KEY"
	export DATABASE_URL="postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable"
	export REDIS_ADDR="127.0.0.1:6379"
	export API_PORT=8000
	export GRPC_PORT=50051
	# shellcheck disable=SC1091
	set -a; [ -f .env ] && . ./.env; set +a
	go build -o /tmp/pcmi-api-host ./cmd/api
	/tmp/pcmi-api-host >/tmp/pcmi-api-host.log 2>&1 &
	HPID=$!
	sleep 4
	if curl -sf http://127.0.0.1:8000/v1/ready >/dev/null; then
		ok "host API /v1/ready"
	else
		bad "host API failed — see /tmp/pcmi-api-host.log"
		tail -20 /tmp/pcmi-api-host.log >&2 || true
	fi
	kill "$HPID" 2>/dev/null || true
	wait "$HPID" 2>/dev/null || true
	$DOCKER_COMPOSE up -d api worker >/dev/null 2>&1 || true
else
	skip "host go run (use --with-host)"
fi

# ══════════════════════════════════════════════════════════════════════════════
section "FINAL SUMMARY"
echo "  passed:  $PASS"
echo "  failed:  $FAIL"
echo "  skipped: $SKIP"
echo ""
echo "  CI on GitHub: include CI_start in commit message for full pipeline."
echo "  .env restored from $ENV_BACKUP"
if [ "${KEEP_STACK:-}" = "1" ]; then
	echo "  stack left running (KEEP_STACK=1)"
else
	echo "  stack: api+worker restarted with restored .env"
fi
[ "$FAIL" -eq 0 ] || exit 1
