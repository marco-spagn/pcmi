#!/usr/bin/env bash
# Exhaustive Cognitive Graph matrix test.
#
# Starts AGE PostgreSQL + Redis + PCMI API, loads
# examples/cognitive-graph-test-matrix/graph_matrix.json through the public HTTP
# API, verifies traversal/chain/Cypher behavior, then cleans up.
set -euo pipefail

GREEN=$'\033[0;32m'
RED=$'\033[0;31m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
RESET=$'\033[0m'

PASSED=0
FAILED=0
TOTAL=0

ok() { printf "  ${GREEN}✓${RESET} %s\n" "$*"; ((PASSED += 1)); ((TOTAL += 1)); }
fail() { printf "  ${RED}✗${RESET} %s\n" "$*" >&2; ((FAILED += 1)); ((TOTAL += 1)); }
info() { printf "${CYAN}→${RESET} %s\n" "$*"; }
header() { printf "\n${BOLD}━━━ %s ━━━${RESET}\n" "$*"; }
hr() { printf "${CYAN}%*s${RESET}\n" "$(tput cols 2>/dev/null || echo 60)" '' | tr ' ' '-'; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DATASET="${DATASET:-$PROJECT_ROOT/examples/cognitive-graph-test-matrix/graph_matrix.json}"
AGE_PORT="${AGE_PORT:-5435}"
AGE_DB="postgres://pcmi:pcmi@localhost:${AGE_PORT}/pcmi?sslmode=disable"
REDIS_PORT="${REDIS_PORT:-6381}"
API_PORT="${API_PORT:-8011}"
GRPC_PORT="${GRPC_PORT:-5011}"
API_URL="http://localhost:${API_PORT}"
API_KEY="${API_KEY:-testkey123}"
REALISTIC_SMOKE_LIMIT="${REALISTIC_SMOKE_LIMIT:-180}"
AGE_CONTAINER="pcmi-postgres-age-matrix"
REDIS_CONTAINER="pcmi-redis-matrix"
IMAGE_TAG="pcmi-postgres-age:matrix-test"
API_BIN="/tmp/pcmi-api-graph-matrix"
ID_MAP="/tmp/pcmi-graph-matrix-ids.tsv"
API_PID=""

cleanup() {
  info "Cleaning up..."
  if [ -n "$API_PID" ] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  if docker info >/dev/null 2>&1; then
    docker rm -f "$AGE_CONTAINER" "$REDIS_CONTAINER" >/dev/null 2>&1 || true
  fi
  rm -f "$API_BIN" "$ID_MAP"
}
trap cleanup EXIT

require_cmds() {
  for cmd in docker curl jq go; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      fail "Missing dependency: $cmd"
      exit 1
    fi
  done
  ok "All dependencies present (docker, curl, jq, go)"
  if ! docker info >/dev/null 2>&1; then
    fail "Docker daemon is not running or not reachable"
    exit 1
  fi
  ok "Docker daemon reachable"
  if [ ! -f "$DATASET" ]; then
    fail "Dataset not found: $DATASET"
    exit 1
  fi
  ok "Dataset found: $DATASET"
}

wait_for_pg() {
  local container="$1" timeout="${2:-90}"
  local deadline=$(( $(date +%s) + timeout ))
  while true; do
    if docker exec "$container" psql -h 127.0.0.1 -U pcmi -d pcmi -c "SELECT 1" >/dev/null 2>&1; then
      return 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      return 1
    fi
    sleep 1
  done
}

wait_for_redis() {
  local container="$1" timeout="${2:-30}"
  local deadline=$(( $(date +%s) + timeout ))
  while true; do
    if docker exec "$container" redis-cli ping >/dev/null 2>&1; then
      return 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      return 1
    fi
    sleep 1
  done
}

wait_for_http() {
  local url="$1" timeout="${2:-45}"
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
}

start_infra() {
  header "Start AGE PostgreSQL and Redis"
  info "Building AGE image..."
  docker build -f "$PROJECT_ROOT/docker/postgres-age/Dockerfile.postgres-age" -t "$IMAGE_TAG" "$PROJECT_ROOT" >/tmp/graph-matrix-docker-build.log 2>&1
  ok "Docker image built: $IMAGE_TAG"

  docker rm -f "$AGE_CONTAINER" "$REDIS_CONTAINER" >/dev/null 2>&1 || true

  local migration_mounts=()
  local migration
  for migration in "$PROJECT_ROOT"/migrations/[0-9][0-9][0-9]_*.sql; do
    case "$(basename "$migration")" in
      019_*) continue ;;
    esac
    migration_mounts+=("-v" "$migration:/docker-entrypoint-initdb.d/$(basename "$migration"):ro")
  done

  docker run -d --name "$AGE_CONTAINER" \
    -e POSTGRES_DB=pcmi \
    -e POSTGRES_USER=pcmi \
    -e POSTGRES_PASSWORD=pcmi \
    -p "${AGE_PORT}:5432" \
    "${migration_mounts[@]}" \
    "$IMAGE_TAG" >/dev/null

  info "Waiting for AGE PostgreSQL on port $AGE_PORT..."
  if wait_for_pg "$AGE_CONTAINER" 90; then
    ok "AGE PostgreSQL ready"
  else
    fail "AGE PostgreSQL did not become ready"
    docker logs "$AGE_CONTAINER" --tail 120 || true
    exit 1
  fi

  docker cp "$PROJECT_ROOT/migrations/019_cognitive_graph_age.sql" "$AGE_CONTAINER:/tmp/019.sql" >/dev/null
  if docker exec "$AGE_CONTAINER" psql -h 127.0.0.1 -U pcmi -d pcmi -f /tmp/019.sql >/tmp/graph-matrix-migration.log 2>&1; then
    ok "AGE migration applied"
  else
    fail "AGE migration failed"
    sed -n '1,120p' /tmp/graph-matrix-migration.log >&2
    exit 1
  fi

  if docker exec "$AGE_CONTAINER" psql -h 127.0.0.1 -U pcmi -d pcmi -c "SELECT 1 FROM ag_catalog.ag_graph LIMIT 1" >/dev/null 2>&1; then
    ok "pcmi_memory_graph exists"
  else
    fail "pcmi_memory_graph missing"
    exit 1
  fi

  docker run -d --name "$REDIS_CONTAINER" -p "${REDIS_PORT}:6379" redis:7-alpine >/dev/null
  if wait_for_redis "$REDIS_CONTAINER" 30; then
    ok "Redis ready"
  else
    fail "Redis did not become ready"
    exit 1
  fi
}

start_api() {
  header "Start PCMI API"
  go build -o "$API_BIN" "$PROJECT_ROOT/cmd/api"
  ok "API binary built"

  DATABASE_URL="postgres://pcmi:pcmi@localhost:${AGE_PORT}/pcmi?sslmode=disable" \
    API_PORT="$API_PORT" \
    GRPC_PORT="$GRPC_PORT" \
    RATE_LIMIT_DISABLED=true \
    REDIS_ADDR="localhost:${REDIS_PORT}" \
    "$API_BIN" >/tmp/graph-matrix-api.log 2>&1 &
  API_PID=$!

  if wait_for_http "$API_URL/v1/ready" 60; then
    ok "API ready at $API_URL"
  else
    fail "API did not become ready"
    sed -n '1,160p' /tmp/graph-matrix-api.log >&2 || true
    exit 1
  fi
}

api_json() {
  local method="$1" path="$2" payload="${3:-}"
  local resp status body
  if [ -n "$payload" ]; then
    resp=$(curl -s -w '\n%{http_code}' --max-time 20 -X "$method" \
      -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
      -d "$payload" "$API_URL$path") || true
  else
    resp=$(curl -s -w '\n%{http_code}' --max-time 20 -X "$method" \
      -H "X-API-Key: $API_KEY" "$API_URL$path") || true
  fi
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | sed '$d')
  if [ "$status" -lt 200 ] || [ "$status" -ge 300 ]; then
    fail "$method $path returned HTTP $status: $body"
    exit 1
  fi
  echo "$body"
}

api_status() {
  local method="$1" path="$2" expected="$3" payload="${4:-}"
  local resp status body
  if [ -n "$payload" ]; then
    resp=$(curl -s -w '\n%{http_code}' --max-time 20 -X "$method" \
      -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
      -d "$payload" "$API_URL$path") || true
  else
    resp=$(curl -s -w '\n%{http_code}' --max-time 20 -X "$method" \
      -H "X-API-Key: $API_KEY" "$API_URL$path") || true
  fi
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | sed '$d')
  if [ "$status" = "$expected" ]; then
    ok "$method $path returned HTTP $expected"
  else
    fail "$method $path expected HTTP $expected, got $status: $body"
  fi
}

id_for() {
  local key="$1"
  awk -F '\t' -v k="$key" '$1 == k { print $2; found=1; exit } END { if (!found) exit 1 }' "$ID_MAP"
}

load_dataset() {
  header "Load graph matrix dataset"
  : > "$ID_MAP"

  local node key path content tags metadata payload body id
  while IFS= read -r node; do
    key=$(jq -r '.key' <<<"$node")
    path=$(jq -r '.path' <<<"$node")
    content=$(jq -r '.content' <<<"$node")
    tags=$(jq -c '.tags' <<<"$node")
    metadata=$(jq -c '.metadata' <<<"$node")
    payload=$(jq -nc --arg path "$path" --arg content "$content" --argjson tags "$tags" --argjson metadata "$metadata" \
      '{path:$path, content:$content, tags:$tags, metadata:$metadata}')
    body=$(api_json POST /v1/memories "$payload")
    id=$(jq -r '.id' <<<"$body")
    if [ -z "$id" ] || [ "$id" = "null" ]; then
      fail "store node $key returned no id: $body"
      exit 1
    fi
    printf "%s\t%s\n" "$key" "$id" >> "$ID_MAP"
  done < <(jq -c '.nodes[]' "$DATASET")

  local node_count
  node_count=$(wc -l < "$ID_MAP" | tr -d ' ')
  ok "Stored $node_count graph nodes"

  local link from_key to_key from_id to_id link_type weight rationale
  while IFS= read -r link; do
    from_key=$(jq -r '.from' <<<"$link")
    to_key=$(jq -r '.to' <<<"$link")
    from_id=$(id_for "$from_key")
    to_id=$(id_for "$to_key")
    link_type=$(jq -r '.type' <<<"$link")
    weight=$(jq -r '.weight' <<<"$link")
    rationale=$(jq -r '.rationale' <<<"$link")
    payload=$(jq -nc \
      --arg from_path "memory.$from_id" \
      --arg to_path "memory.$to_id" \
      --arg link_type "$link_type" \
      --arg rationale "$rationale" \
      --argjson weight "$weight" \
      '{from_path:$from_path, to_path:$to_path, link_type:$link_type, metadata:{rationale:$rationale, weight:$weight}}')
    api_json POST /v1/memories/links "$payload" >/dev/null
  done < <(jq -c '.links[]' "$DATASET")

  ok "Created $(jq '.links | length' "$DATASET") graph links"
  sleep 2
}

assert_eq() {
  local got="$1" want="$2" label="$3"
  if [ "$got" = "$want" ]; then
    ok "$label"
  else
    fail "$label: wanted $want, got $got"
  fi
}

assert_related_contains() {
  local body="$1" key="$2" id
  id=$(id_for "$key")
  if jq -e --argjson id "$id" 'any(.entries[]?; .id == $id)' <<<"$body" >/dev/null; then
    ok "related contains $key ($id)"
  else
    fail "related missing $key ($id): $(jq -c '{count,total,entries}' <<<"$body")"
  fi
}

assert_related_not_contains() {
  local body="$1" key="$2" id
  id=$(id_for "$key")
  if jq -e --argjson id "$id" 'any(.entries[]?; .id == $id)' <<<"$body" >/dev/null; then
    fail "related unexpectedly contains $key ($id): $(jq -c '{count,total,entries}' <<<"$body")"
  else
    ok "related excludes $key ($id)"
  fi
}

assert_chain() {
  local from_key="$1" to_key="$2" link_types="$3" want_connected="$4" want_hops="$5"
  local from_id to_id body connected hops
  from_id=$(id_for "$from_key")
  to_id=$(id_for "$to_key")
  body=$(api_json GET "/v1/graph/chain?from=${from_id}&to=${to_id}&link_types=${link_types}&max_depth=8")
  connected=$(jq -r '.connected' <<<"$body")
  hops=$(jq -r '.hops' <<<"$body")
  assert_eq "$connected" "$want_connected" "chain $from_key -> $to_key connected=$want_connected"
  if [ "$want_connected" = "true" ]; then
    assert_eq "$hops" "$want_hops" "chain $from_key -> $to_key hops=$want_hops"
  fi
}

run_assertions() {
  header "Run graph matrix assertions"
  local root fanout isolated cycle_a self_loop body count total next page1 page2 p1_ids p2_ids id1 id2 rows

  body=$(api_json GET /v1/graph/health)
  assert_eq "$(jq -r '.available' <<<"$body")" "true" "graph health available"

  root=$(id_for root)
  body=$(api_json GET "/v1/graph/related?memory_id=${root}&depth=3&limit=20")
  assert_eq "$(jq -r '.total' <<<"$body")" "7" "root all-link traversal total with duplicate path dedup"
  for key in causal_1 temporal_1 supports_1 related_1 contradict_1 causal_2 alternate_target; do
    assert_related_contains "$body" "$key"
  done

  body=$(api_json GET "/v1/graph/related?memory_id=${root}&depth=3&link_types=causal,temporal,supports&limit=20")
  assert_eq "$(jq -r '.total' <<<"$body")" "4" "root allowed-type traversal total"
  for key in causal_1 temporal_1 supports_1 causal_2; do
    assert_related_contains "$body" "$key"
  done
  for key in related_1 contradict_1 alternate_target; do
    assert_related_not_contains "$body" "$key"
  done

  body=$(api_json GET "/v1/graph/related?memory_id=${root}&depth=3&link_types=causal&limit=20")
  assert_eq "$(jq -r '.total' <<<"$body")" "2" "root causal-only traversal total"
  assert_related_contains "$body" causal_1
  assert_related_contains "$body" causal_2
  assert_related_not_contains "$body" temporal_1

  fanout=$(id_for fanout_hub)
  page1=$(api_json GET "/v1/graph/related?memory_id=${fanout}&depth=1&link_types=supports&limit=2")
  count=$(jq -r '.count' <<<"$page1")
  total=$(jq -r '.total' <<<"$page1")
  next=$(jq -r '.next_cursor' <<<"$page1")
  assert_eq "$count" "2" "fanout page 1 count"
  assert_eq "$total" "4" "fanout total"
  if [ "$next" = "0" ] || [ "$next" = "null" ]; then
    fail "fanout page 1 should return next_cursor"
  else
    ok "fanout page 1 returns next_cursor"
  fi
  page2=$(api_json GET "/v1/graph/related?memory_id=${fanout}&depth=1&link_types=supports&limit=2&cursor=${next}")
  assert_eq "$(jq -r '.count' <<<"$page2")" "2" "fanout page 2 count"
  p1_ids=$(jq -r '.entries[].id' <<<"$page1")
  p2_ids=$(jq -r '.entries[].id' <<<"$page2")
  for id1 in $p1_ids; do
    for id2 in $p2_ids; do
      if [ "$id1" = "$id2" ]; then
        fail "fanout pagination overlap on id $id1"
      fi
    done
  done
  ok "fanout pagination has no overlap"

  isolated=$(id_for isolated)
  body=$(api_json GET "/v1/graph/related?memory_id=${isolated}&depth=5&limit=20")
  assert_eq "$(jq -r '.count' <<<"$body")" "0" "isolated node has no related entries"

  cycle_a=$(id_for cycle_a)
  body=$(api_json GET "/v1/graph/related?memory_id=${cycle_a}&depth=1&link_types=related&limit=20")
  assert_eq "$(jq -r '.total' <<<"$body")" "1" "cycle depth=1 total"
  assert_related_contains "$body" cycle_b
  assert_related_not_contains "$body" cycle_a

  body=$(api_json GET "/v1/graph/related?memory_id=${cycle_a}&depth=2&link_types=related&limit=20")
  assert_eq "$(jq -r '.total' <<<"$body")" "2" "cycle depth=2 total"
  assert_related_contains "$body" cycle_b
  assert_related_contains "$body" cycle_a

  self_loop=$(id_for self_loop)
  body=$(api_json GET "/v1/graph/related?memory_id=${self_loop}&depth=1&link_types=related&limit=20")
  assert_eq "$(jq -r '.total' <<<"$body")" "1" "self-loop traversal total"
  assert_related_contains "$body" self_loop

  assert_chain root supports_1 "causal,temporal,supports" true 1
  assert_chain causal_1 supports_1 "temporal,supports" true 2
  assert_chain root supports_1 "causal" false 0
  assert_chain root alternate_target "causal,contradicts" true 2
  assert_chain supports_1 root "causal,temporal,supports,related" false 0
  assert_chain fanout_hub support_d "supports" true 1
  assert_chain self_loop self_loop "related" true 1

  body=$(api_json POST /v1/graph/cypher '{"query":"MATCH (n:Memory) WHERE n.id IS NOT NULL RETURN n.id ORDER BY n.id LIMIT 5"}')
  rows=$(jq '.rows | length' <<<"$body")
  if [ "$rows" -gt 0 ]; then
    ok "Cypher passthrough with existing WHERE returns rows"
  else
    fail "Cypher passthrough returned no rows"
  fi

  api_status POST /v1/graph/cypher 400 '{"query":"MATCH (n:Memory) DELETE n"}'
  api_status POST /v1/graph/cypher 400 '{"query":"MATCH (n:Memory) SET\nn.x = 1 RETURN n"}'
  api_status POST /v1/graph/cypher 400 '{"query":"MATCH (n:Memory) RETURN n INSERT INTO memory_links VALUES (1)"}'
  api_status POST /v1/graph/cypher 400 '{"query":"MATCH (n:Person) RETURN n"}'
  api_status POST /v1/graph/cypher 400 '{"query":"MATCH (n:Memory) RETURN n $$) AS (x ag_catalog.agtype) SELECT 1"}'
  api_status POST /v1/graph/cypher 400 '{"query":"MATCH (n:Memory) RETURN n; MATCH (m:Memory) RETURN m"}'
  api_status GET /v1/graph/related 400
  api_status GET "/v1/graph/chain?from=abc&to=1" 400
}

run_age_partial_setup_assertions() {
  header "Run AGE partial-setup assertions"
  docker exec "$AGE_CONTAINER" psql -h 127.0.0.1 -U pcmi -d pcmi \
    -c "SET search_path = ag_catalog, public; SELECT drop_graph('pcmi_memory_graph', true);" >/tmp/graph-matrix-drop-graph.log 2>&1

  local body
  body=$(api_json GET /v1/graph/health)
  assert_eq "$(jq -r '.available' <<<"$body")" "false" "graph health false when pcmi_memory_graph is missing"
  api_status GET "/v1/graph/related?memory_id=1" 501
  api_status GET "/v1/graph/chain?from=1&to=2" 501
  api_status POST /v1/graph/cypher 501 '{"query":"MATCH (n:Memory) RETURN n LIMIT 1"}'
}

run_graph_integration_tests() {
  header "Run graph integration tests against AGE"
  local pattern
  pattern='TestIntegration_(FindRelated_DeduplicatesMultiplePaths|FindRelated_NumericPaginationBoundary|ExecuteCypher_MultiTenantIsolation|MemoryLinkDeleteRemovesAGEEdge)$'
  if DATABASE_URL="$AGE_DB" go test -tags=integration ./internal/graph -run "$pattern" -count=1 >/tmp/graph-matrix-integration.log 2>&1; then
    ok "Graph integration tests passed"
  else
    fail "Graph integration tests failed"
    sed -n '1,180p' /tmp/graph-matrix-integration.log >&2 || true
    exit 1
  fi
}

run_realistic_load_smoke() {
  header "Run realistic graph load smoke"
  if PCMI_BASE_URL="$API_URL" PCMI_API_KEY="$API_KEY" \
    python3 "$PROJECT_ROOT/examples/cognitive-graph-realistic/smoke_load_to_pcmi.py" \
      --limit "$REALISTIC_SMOKE_LIMIT" >/tmp/graph-realistic-load-smoke.log 2>&1; then
    ok "Realistic graph load smoke passed (${REALISTIC_SMOKE_LIMIT} nodes)"
  else
    fail "Realistic graph load smoke failed"
    sed -n '1,180p' /tmp/graph-realistic-load-smoke.log >&2 || true
    exit 1
  fi
}

main() {
  header "Cognitive Graph test matrix"
  require_cmds
  start_infra
  start_api
  load_dataset
  run_assertions
  run_graph_integration_tests
  run_realistic_load_smoke
  run_age_partial_setup_assertions

  header "Results"
  echo "  Total:  $TOTAL"
  echo "  Passed: ${GREEN}$PASSED${RESET}"
  echo "  Failed: ${RED}$FAILED${RESET}"
  if [ "$FAILED" -gt 0 ]; then
    exit 1
  fi
  echo "  ${GREEN}All graph matrix checks passed${RESET}"
}

main "$@"
