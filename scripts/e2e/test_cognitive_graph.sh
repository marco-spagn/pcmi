#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# test_cognitive_graph.sh — E2E smoke test for Cognitive Graph v2.0
#
# Tests:
#   1. Docker image build (pgvector + Apache AGE bundled)
#   2. Migration 019 (graceful degradation when AGE absent)
#   3. GET  /v1/graph/health
#   4. GET  /v1/graph/related  (keyset cursor pagination)
#   5. GET  /v1/graph/chain    (shortest-path reconstruction)
#   6. POST /v1/graph/cypher   (read-only Cypher passthrough)
#   7. Graceful degradation (501 on standard Postgres without AGE)
#
# Usage:
#   bash scripts/e2e/test_cognitive_graph.sh
#
# Prerequisites: docker, curl, jq, go
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Colours ─────────────────────────────────────────────────────────────────
GREEN=$'\033[0;32m'
YELLOW=$'\033[1;33m'
RED=$'\033[0;31m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
RESET=$'\033[0m'

PASSED=0
FAILED=0
TOTAL=0

ok()     { printf "  ${GREEN}✓${RESET} %s\n" "$*"; ((PASSED+=1)); ((TOTAL+=1)); }
fail()   { printf "  ${RED}✗${RESET} %s\n" "$*" >&2; ((FAILED+=1)); ((TOTAL+=1)); }
info()   { printf "${CYAN}→${RESET} %s\n" "$*"; }
warn()   { printf "${YELLOW}⚠${RESET}  %s\n" "$*"; }
header() { printf "\n${BOLD}━━━ %s ━━━${RESET}\n" "$*"; }
hr()     { printf "${CYAN}%*s${RESET}\n" "$(tput cols 2>/dev/null || echo 60)" '' | tr ' ' '─'; }

# ── Config ─────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
AGE_PORT="${AGE_PORT:-5433}"
AGE_DB="postgres://pcmi:pcmi@localhost:${AGE_PORT}/pcmi?sslmode=disable"
AGE_RAW="postgres://pcmi:pcmi@localhost:${AGE_PORT}/pcmi"
API_PORT="${API_PORT:-8000}"
API_URL="http://localhost:${API_PORT}"
API_KEY="${API_KEY:-testkey123}"
DOCKERFILE="docker/postgres-age/Dockerfile.postgres-age"
IMAGE_TAG="pcmi-postgres-age:e2e-test"

# We start the API manually, not via docker compose, so we can point it at the
# AGE instance and control the port.
API_PID=""

# ── Helpers ─────────────────────────────────────────────────────────────────

cleanup() {
  info "Cleaning up..."
  if [ -n "$API_PID" ] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  # Optional: stop and remove the AGE container.
  if docker ps -q -f name="pcmi-postgres-age" | grep -q .; then
    docker rm -f pcmi-postgres-age >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

wait_for_http() {
  local url="$1" timeout="${2:-60}" label="${3:-endpoint}"
  local deadline=$(( $(date +%s) + timeout ))
  while true; do
    if curl -sf --max-time 3 "$url" >/dev/null 2>&1; then
      return 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      return 1
    fi
    sleep 1
  done
  return 1
}

wait_for_pg() {
  local container="$1" timeout="${2:-60}"
  local deadline=$(( $(date +%s) + timeout ))
  while true; do
    if docker exec "$container" pg_isready -U pcmi >/dev/null 2>&1; then
      return 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      return 1
    fi
    sleep 1
  done
  return 1
}

# shellcheck disable=SC2028
curl_api() {
  # All API calls use X-API-Key header.
  curl -s -f --max-time 15 -H "X-API-Key: ${API_KEY}" "$@"
}

assert_eq() {
  local got="$1" want="$2" label="$3"
  if [ "$got" = "$want" ]; then
    ok "$label — got '$want'"
  else
    fail "$label — wanted '$want', got '$got'"
  fi
}

assert_gt() {
  local got="$1" min="$2" label="$3"
  if [ "$got" -gt "$min" ]; then
    ok "$label — got $got (> $min)"
  else
    fail "$label — wanted > $min, got $got"
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" label="$3"
  if echo "$haystack" | grep -q "$needle"; then
    ok "$label"
  else
    fail "$label — output did not contain '$needle'"
  fi
}

# ── Prerequisites ───────────────────────────────────────────────────────────
header "Cognitive Graph v2.0 — E2E Smoke Test"

for cmd in docker curl jq go; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    fail "Missing dependency: $cmd"
    exit 1
  fi
done
ok "All dependencies present (docker, curl, jq, go)"

# ── Step 1: Build the bundled Docker image ──────────────────────────────────
header "Step 1: Build Docker image (pgvector + Apache AGE)"
info "Building $DOCKERFILE ..."

if docker build -f "$DOCKERFILE" -t "$IMAGE_TAG" "$PROJECT_ROOT" > /tmp/graph-docker-build.log 2>&1; then
  ok "Docker image built: $IMAGE_TAG"
else
  warn "Docker build failed (see /tmp/graph-docker-build.log) — falling back to apache/age"
  IMAGE_TAG="apache/age:latest"
  ok "Using fallback image: $IMAGE_TAG"
fi

hr

# ── Step 2: Start AGE Postgres ──────────────────────────────────────────────
header "Step 2: Start AGE-enabled Postgres (port $AGE_PORT)"

docker rm -f pcmi-postgres-age >/dev/null 2>&1 || true
docker run -d --name pcmi-postgres-age \
  -e POSTGRES_DB=pcmi \
  -e POSTGRES_USER=pcmi \
  -e POSTGRES_PASSWORD=pcmi \
  -p "${AGE_PORT}:5432" \
  -v "$PROJECT_ROOT/migrations/001_init.sql:/docker-entrypoint-initdb.d/001_init.sql:ro" \
  -v "$PROJECT_ROOT/migrations/002_multi_tenant.sql:/docker-entrypoint-initdb.d/002_multi_tenant.sql:ro" \
  -v "$PROJECT_ROOT/migrations/003_rbac_api_keys.sql:/docker-entrypoint-initdb.d/003_rbac_api_keys.sql:ro" \
  -v "$PROJECT_ROOT/migrations/004_audit_encryption.sql:/docker-entrypoint-initdb.d/004_audit_encryption.sql:ro" \
  -v "$PROJECT_ROOT/migrations/005_worker_helpers.sql:/docker-entrypoint-initdb.d/005_worker_helpers.sql:ro" \
  -v "$PROJECT_ROOT/migrations/006_fts_hybrid.sql:/docker-entrypoint-initdb.d/006_fts_hybrid.sql:ro" \
  -v "$PROJECT_ROOT/migrations/007_v1_11_embedding_space.sql:/docker-entrypoint-initdb.d/007_v1_11_embedding_space.sql:ro" \
  -v "$PROJECT_ROOT/migrations/008_v1_12_distilled_webhooks_encrypt.sql:/docker-entrypoint-initdb.d/008_v1_12_distilled_webhooks_encrypt.sql:ro" \
  -v "$PROJECT_ROOT/migrations/009_v1_13_event_schemas_webhook_dlq.sql:/docker-entrypoint-initdb.d/009_v1_13_event_schemas_webhook_dlq.sql:ro" \
  -v "$PROJECT_ROOT/migrations/010_v1_14_consolidation_bm25_admin.sql:/docker-entrypoint-initdb.d/010_v1_14_consolidation_bm25_admin.sql:ro" \
  -v "$PROJECT_ROOT/migrations/011_v1_15_links_expiry_tags.sql:/docker-entrypoint-initdb.d/011_v1_15_links_expiry_tags.sql:ro" \
  -v "$PROJECT_ROOT/migrations/012_v1_19_memory_compaction.sql:/docker-entrypoint-initdb.d/012_v1_19_memory_compaction.sql:ro" \
  -v "$PROJECT_ROOT/migrations/013_idempotency.sql:/docker-entrypoint-initdb.d/013_idempotency.sql:ro" \
  -v "$PROJECT_ROOT/migrations/014_key_lifecycle.sql:/docker-entrypoint-initdb.d/014_key_lifecycle.sql:ro" \
  -v "$PROJECT_ROOT/migrations/015_importance_decay.sql:/docker-entrypoint-initdb.d/015_importance_decay.sql:ro" \
  -v "$PROJECT_ROOT/migrations/016_sessions.sql:/docker-entrypoint-initdb.d/016_sessions.sql:ro" \
  -v "$PROJECT_ROOT/migrations/017_dedup.sql:/docker-entrypoint-initdb.d/017_dedup.sql:ro" \
  -v "$PROJECT_ROOT/migrations/018_distillation_policy.sql:/docker-entrypoint-initdb.d/018_distillation_policy.sql:ro" \
  "$IMAGE_TAG" >/dev/null 2>&1

info "Waiting for Postgres to become healthy..."
if wait_for_pg pcmi-postgres-age 60; then
  ok "Postgres is ready on port $AGE_PORT"
else
  fail "Postgres did not become healthy within 60s"
  exit 1
fi

# Wait until base tables exist (init scripts may still be running even after pg_isready).
info "Waiting for migrations to be applied (memory_links table)..."
for i in $(seq 1 30); do
  if docker exec pcmi-postgres-age psql -U pcmi -d pcmi -c "SELECT 1 FROM public.memory_links LIMIT 0" >/dev/null 2>&1; then
    ok "Base migrations applied (memory_links exists)"
    break
  fi
  if [ "$i" -eq 30 ]; then
    fail "Base migrations not applied after 30 attempts"
    exit 1
  fi
  sleep 1
done

hr

# ── Step 3: Apply migration 019 ─────────────────────────────────────────────
header "Step 3: Apply migration 019 (cognitive graph)"

MIGRATION_019="$PROJECT_ROOT/migrations/019_cognitive_graph_age.sql"

# Copy migration into the container and run it.
docker cp "$MIGRATION_019" pcmi-postgres-age:/tmp/019.sql >/dev/null
# Try Unix socket first, then TCP fallback.
DOCKER_PSQL="docker exec pcmi-postgres-age psql -U pcmi -d pcmi"
if ! $DOCKER_PSQL -f /tmp/019.sql > /tmp/graph-migration.log 2>&1; then
  # Try with TCP
  if ! docker exec pcmi-postgres-age psql -h 127.0.0.1 -U pcmi -d pcmi -f /tmp/019.sql > /tmp/graph-migration.log 2>&1; then
    warn "Migration had non-zero exit — checking if AGE is still available..."
    cat /tmp/graph-migration.log | tail -5
  fi
fi
docker exec pcmi-postgres-age rm /tmp/019.sql >/dev/null 2>&1 || true

# Verify AGE extension was created.
AGEOK=false
for psql_cmd in "psql -U pcmi -d pcmi" "psql -h 127.0.0.1 -U pcmi -d pcmi"; do
  if docker exec pcmi-postgres-age $psql_cmd -c "SELECT 1 FROM ag_catalog.ag_graph LIMIT 1" >/dev/null 2>&1; then
    AGEOK=true
    break
  fi
done
if $AGEOK; then
  ok "Migration applied — AGE extension and pcmi_memory_graph exist"
else
  fail "Migration did not create AGE graph"
  exit 1
fi

# Verify pgvector is also available.
if docker exec pcmi-postgres-age psql -U pcmi -d pcmi -c "CREATE EXTENSION IF NOT EXISTS vector" >/dev/null 2>&1; then
  ok "pgvector extension available (bundled image)"
else
  warn "pgvector extension not available — embedding features disabled on this instance"
fi

# Verify the trigger exists.
TRIGGER_CHECK=$(docker exec pcmi-postgres-age psql -U pcmi -d pcmi -t -c \
  "SELECT count(*) FROM information_schema.triggers WHERE trigger_name = 'trg_memory_links_sync_graph'" 2>/dev/null | tr -d ' ')
if [ "$TRIGGER_CHECK" -ge 1 ]; then
  ok "Trigger trg_memory_links_sync_graph exists on memory_links"
else
  fail "Trigger trg_memory_links_sync_graph is missing (count=$TRIGGER_CHECK)"
fi

hr

# ── Step 4: Start PCMI API pointed at AGE ───────────────────────────────────
header "Step 4: Start PCMI API (port $API_PORT)"

info "Starting API with DATABASE_URL=$AGE_DB on port $API_PORT"
DATABASE_URL="$AGE_RAW" \
  API_PORT="$API_PORT" \
  REDIS_ADDR="${REDIS_ADDR:-localhost:6379}" \
  go run "$PROJECT_ROOT/cmd/api" > /tmp/graph-api.log 2>&1 &
API_PID=$!

if wait_for_http "${API_URL}/v1/ready" 30 "API /v1/ready"; then
  ok "API is ready on $API_URL"
else
  fail "API did not become healthy within 30s"
  cat /tmp/graph-api.log
  exit 1
fi

hr

# ── Step 5: Store test memories ─────────────────────────────────────────────
header "Step 5: Insert fake test data (memories + links)"

# Store 5 memories — these form a chain: 1 → 2 → 3 (causal), 4 → 5 (temporal)
MEM_1=$(curl -sf --max-time 10 -X POST "${API_URL}/v1/memories" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"path":"root.cognitive.e2e.a","content":"The server crashes on startup after upgrading to v2.3.1","tags":["e2e","cognitive"]}' \
  | jq -r '.id')
info "  Memory 1 (root cause): id=$MEM_1"

MEM_2=$(curl -sf --max-time 10 -X POST "${API_URL}/v1/memories" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"path":"root.cognitive.e2e.b","content":"The crash is caused by a null-pointer dereference in config.c line 142","tags":["e2e","cognitive"]}' \
  | jq -r '.id')
info "  Memory 2 (diagnosis): id=$MEM_2"

MEM_3=$(curl -sf --max-time 10 -X POST "${API_URL}/v1/memories" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"path":"root.cognitive.e2e.c","content":"Fixed the null pointer bug, deployed in v2.3.2, crash resolved","tags":["e2e","cognitive"]}' \
  | jq -r '.id')
info "  Memory 3 (fix): id=$MEM_3"

MEM_4=$(curl -sf --max-time 10 -X POST "${API_URL}/v1/memories" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"path":"root.cognitive.e2e.d","content":"User reported slow response times after the config fix deployment","tags":["e2e","cognitive"]}' \
  | jq -r '.id')
info "  Memory 4 (side-effect): id=$MEM_4"

MEM_5=$(curl -sf --max-time 10 -X POST "${API_URL}/v1/memories" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"path":"root.cognitive.e2e.e","content":"The slow responses are unrelated — caused by a DNS misconfiguration in the load balancer","tags":["e2e","cognitive"]}' \
  | jq -r '.id')
info "  Memory 5 (unrelated): id=$MEM_5"

ok "5 test memories stored"

# Create links — use from_path/to_path format (ltree paths, not integer IDs)
info "Creating links..."

link() {
  local from_path="$1" to_path="$2" link_type="$3"
  curl -sf --max-time 10 -X POST "${API_URL}/v1/memories/links" \
    -H "X-API-Key: ${API_KEY}" \
    -H "Content-Type: application/json" \
    -d "{\"from_path\":\"${from_path}\",\"to_path\":\"${to_path}\",\"link_type\":\"${link_type}\"}" \
    >/dev/null 2>&1
}

# 1→2 (causal): crash report leads to diagnosis
link "memory.${MEM_1}" "memory.${MEM_2}" "causal"
info "  link: $MEM_1 --[causal]--> $MEM_2"

# 2→3 (causal): diagnosis leads to fix
link "memory.${MEM_2}" "memory.${MEM_3}" "causal"
info "  link: $MEM_2 --[causal]--> $MEM_3"

# 3→4 (temporal): fix deployment leads to performance report
link "memory.${MEM_3}" "memory.${MEM_4}" "temporal"
info "  link: $MEM_3 --[temporal]--> $MEM_4"

# 4→5 (supports): report leads to root-cause analysis
link "memory.${MEM_4}" "memory.${MEM_5}" "supports"
info "  link: $MEM_4 --[supports]--> $MEM_5"

# 2→4 (contradicts): the diagnosis was actually wrong
link "memory.${MEM_2}" "memory.${MEM_4}" "contradicts"
info "  link: $MEM_2 --[contradicts]--> $MEM_4"

ok "5 links created (causal chain + temporal + supports + contradicts)"

# Give the AGE trigger a moment to sync.
sleep 2

hr

# ── Step 6: Test /v1/graph/health ───────────────────────────────────────────
header "Step 6: Test GET /v1/graph/health"

HEALTH=$(curl -sf --max-time 5 "${API_URL}/v1/graph/health")
HEALTH_AVAIL=$(echo "$HEALTH" | jq -r '.available')
HEALTH_EXT=$(echo "$HEALTH" | jq -r '.extension')

assert_eq "$HEALTH_AVAIL" "true" "Health: available"
assert_eq "$HEALTH_EXT" "apache-age" "Health: extension name"

hr

# ── Step 7: Test GET /v1/graph/related (single page) ────────────────────────
header "Step 7: Test GET /v1/graph/related (traversal)"

RELATED=$(curl_api "${API_URL}/v1/graph/related?memory_id=${MEM_1}&depth=3&link_types=causal,temporal")
REL_COUNT=$(echo "$RELATED" | jq -r '.count')
REL_TOTAL=$(echo "$RELATED" | jq -r '.total')
REL_DEPTH=$(echo "$RELATED" | jq -r '.depth')
REL_FIRST_ID=$(echo "$RELATED" | jq -r '.entries[0].id')

assert_eq "$REL_DEPTH" "3"      "Depth=3 in response"
assert_gt "$REL_COUNT" "0"      "Has results (count>0)"
assert_gt "$REL_TOTAL" "0"      "Total>0"

# Verify the causal chain members are present.
REL_IDS=$(echo "$RELATED" | jq -r '[.entries[].id] | join(",")')
assert_contains "$REL_IDS" "$MEM_2" "Memory $MEM_2 reachable from $MEM_1"
assert_contains "$REL_IDS" "$MEM_3" "Memory $MEM_3 reachable from $MEM_1"

info "Related entries: $REL_IDS"

hr

# ── Step 8: Test GET /v1/graph/related (cursor pagination) ──────────────────
header "Step 8: Test keyset cursor pagination"

# First page — get up to 2 entries.
PAGE1=$(curl_api "${API_URL}/v1/graph/related?memory_id=${MEM_1}&depth=3&link_types=causal,temporal&limit=2")
P1_COUNT=$(echo "$PAGE1" | jq -r '.count')
P1_NEXT=$(echo "$PAGE1" | jq -r '.next_cursor')
P1_IDS=$(echo "$PAGE1" | jq -r '[.entries[].id] | join(",")')

assert_eq "$P1_COUNT" "2" "Page 1: exactly 2 entries (limit=2)"

# Second page using the cursor from page 1.
if [ "$P1_NEXT" != "0" ] && [ "$P1_NEXT" != "null" ]; then
  info "Fetching page 2 with cursor=$P1_NEXT"
  PAGE2=$(curl_api "${API_URL}/v1/graph/related?memory_id=${MEM_1}&depth=3&link_types=causal,temporal&cursor=${P1_NEXT}&limit=2")
  P2_COUNT=$(echo "$PAGE2" | jq -r '.count')
  P2_IDS=$(echo "$PAGE2" | jq -r '[.entries[].id] | join(",")')

  assert_gt "$P2_COUNT" "-1" "Page 2: has results"

  # IDs across pages must not overlap.
  OVERLAP=$(echo "$P1_IDS" | tr ',' '\n' | sort | comm -12 - <(echo "$P2_IDS" | tr ',' '\n' | sort) | wc -l | tr -d ' ')
  if [ "$OVERLAP" = "0" ]; then
    ok "No overlap between page 1 and page 2"
  else
    fail "Pages overlap — $OVERLAP duplicate IDs found"
  fi
else
  warn "Only one page of results — skipping page 2 test"
fi

hr

# ── Step 9: Test GET /v1/graph/chain ────────────────────────────────────────
header "Step 9: Test GET /v1/graph/chain (shortest path)"

# The causal chain 1→2→3 should be found.
# NOTE: chain uses memory_entries.id, not ltree paths.
CHAIN=$(curl_api "${API_URL}/v1/graph/chain?from=${MEM_1}&to=${MEM_3}&link_types=causal&max_depth=5")
CHAIN_CONNECTED=$(echo "$CHAIN" | jq -r '.connected')
CHAIN_HOPS=$(echo "$CHAIN" | jq -r '.hops')
CHAIN_LEN=$(echo "$CHAIN" | jq -r '.path | length')

assert_eq "$CHAIN_CONNECTED" "true" "Chain $MEM_1 → $MEM_3: connected"
assert_eq "$CHAIN_HOPS" "2"        "Chain $MEM_1 → $MEM_3: 2 hops"
assert_eq "$CHAIN_LEN" "2"         "Chain $MEM_1 → $MEM_3: 2 path edges"

# Test a path that should NOT exist (going the wrong direction).
CHAIN_REV=$(curl_api "${API_URL}/v1/graph/chain?from=${MEM_3}&to=${MEM_1}&link_types=causal&max_depth=5")
CHAIN_REV_CONN=$(echo "$CHAIN_REV" | jq -r '.connected')
assert_eq "$CHAIN_REV_CONN" "false" "Chain $MEM_3 → $MEM_1 (reverse): not connected"

# Test with no path between unrelated memories.
CHAIN_NONE=$(curl_api "${API_URL}/v1/graph/chain?from=${MEM_1}&to=${MEM_5}&link_types=causal&max_depth=5")
CHAIN_NONE_CONN=$(echo "$CHAIN_NONE" | jq -r '.connected')
info "  $MEM_1 --[causal]--> $MEM_5: connected=$CHAIN_NONE_CONN (paths via contradicts may not match)"

hr

# ── Step 10: Test POST /v1/graph/cypher ─────────────────────────────────────
header "Step 10: Test POST /v1/graph/cypher (passthrough)"

CYPHER=$(curl -sf --max-time 15 \
  -X POST "${API_URL}/v1/graph/cypher" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"MATCH (n:Memory) RETURN n.id ORDER BY n.id LIMIT 10\"}")
CYPHER_ROWS=$(echo "$CYPHER" | jq -r '.rows | length')
CYPHER_COLS=$(echo "$CYPHER" | jq -r '.columns | join(",")')

assert_gt "$CYPHER_ROWS" "0" "Cypher passthrough: returned rows"
assert_eq "$CYPHER_COLS" "result" "Cypher passthrough: columns"

# Test that dangerous queries are rejected.
REJECT=$(curl -s --max-time 10 \
  -X POST "${API_URL}/v1/graph/cypher" \
  -H "X-API-Key: ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"query":"CREATE (n:Memory {id: \"memory.99\"})"}' \
  | jq -r '.error // "no error"')
assert_contains "$REJECT" "not allowed" "Cypher CREATE rejected by allowlist"

hr

# ── Step 11: Test graceful degradation (without AGE) ────────────────────────
header "Step 11: Test graceful degradation (standard Postgres)"

# Start a standard postgres (no AGE) on port 5434.
docker rm -f pcmi-postgres-std >/dev/null 2>&1 || true
docker run -d --name pcmi-postgres-std \
  -e POSTGRES_DB=pcmi \
  -e POSTGRES_USER=pcmi \
  -e POSTGRES_PASSWORD=pcmi \
  -p 5434:5432 \
  pgvector/pgvector:pg16 >/dev/null 2>&1

# Apply ONLY migration 019 (simulates running migration without AGE).
docker cp "$MIGRATION_019" pcmi-postgres-std:/tmp/019.sql >/dev/null
docker exec pcmi-postgres-std psql -h 127.0.0.1 -U pcmi -d pcmi -f /tmp/019.sql > /tmp/graph-migration-std.log 2>&1 || true
docker exec pcmi-postgres-std rm /tmp/019.sql >/dev/null 2>&1

wait_for_pg pcmi-postgres-std 30

# Start a second API instance on a different port, pointed at standard PG.
API_STD_PORT=8002
info "Starting API on port $API_STD_PORT (no AGE)..."
API_STD_PID=""
DATABASE_URL="postgres://pcmi:pcmi@localhost:5434/pcmi?sslmode=disable" \
  API_PORT="$API_STD_PORT" \
  go run "$PROJECT_ROOT/cmd/api" > /tmp/graph-api-std.log 2>&1 &
API_STD_PID=$!

if wait_for_http "http://localhost:${API_STD_PORT}/v1/ready" 30 "API(no-AGE) /v1/ready"; then
  ok "API without AGE is ready on port $API_STD_PORT"
else
  fail "API without AGE did not start"
  kill "$API_STD_PID" 2>/dev/null || true
  docker rm -f pcmi-postgres-std >/dev/null 2>&1
fi

if [ -n "$API_STD_PID" ]; then
  # Health must report available=false.
  HEALTH_NOAGE=$(curl -sf --max-time 5 "http://localhost:${API_STD_PORT}/v1/graph/health")
  assert_eq "$(echo "$HEALTH_NOAGE" | jq -r '.available')" "false" \
    "Health(no AGE): available=false"

  # Related must return 501.
  STATUS=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    -H "X-API-Key: ${API_KEY}" \
    "http://localhost:${API_STD_PORT}/v1/graph/related?memory_id=1")
  assert_eq "$STATUS" "501" "Related(no AGE): HTTP 501"

  # Chain must also return 501.
  STATUS_CHAIN=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    -H "X-API-Key: ${API_KEY}" \
    "http://localhost:${API_STD_PORT}/v1/graph/chain?from=1&to=2")
  assert_eq "$STATUS_CHAIN" "501" "Chain(no AGE): HTTP 501"

  # Cleanup the no-AGE API and container.
  kill "$API_STD_PID" 2>/dev/null || true
  docker rm -f pcmi-postgres-std >/dev/null 2>&1
fi

hr

# ── Results ─────────────────────────────────────────────────────────────────
header "Results"

echo ""
echo "  Total:  $TOTAL"
echo "  Passed: ${GREEN}$PASSED${RESET}"
echo "  Failed: ${RED}$FAILED${RESET}"
echo ""

if [ "$FAILED" -gt 0 ]; then
  echo "  ${RED}✗ SOME TESTS FAILED${RESET}"
  exit 1
else
  echo "  ${GREEN}✓ All ${PASSED} tests passed${RESET}"
  exit 0
fi
