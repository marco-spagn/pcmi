#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# seed_graph_data.sh — Populate the Cognitive Graph with demo data.
#
# Creates ~25 interconnected memories across 5 scenarios using all 5 link types
# (causal, temporal, contradicts, supports, related).
#
# Usage:
#   API_URL=http://localhost:8000 API_KEY=testkey123 bash scripts/e2e/seed_graph_data.sh
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail

API="${API_URL:-http://localhost:8000}/v1"
KEY="${API_KEY:-testkey123}"

GREEN=$'\033[0;32m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'
RED=$'\033[0;31m'
RESET=$'\033[0m'

ok()   { printf "  ${GREEN}OK${RESET}  %s\n" "$*"; }
info() { printf "${CYAN}  ->${RESET} %s\n" "$*"; }
hdr()  { printf "\n${BOLD}%s${RESET}\n" "$*"; }
warn() { printf "  ${RED}!!${RESET} %s\n" "$*"; }

# ── helpers ─────────────────────────────────────────────────────────────────
# ltree labels only allow A-Za-z0-9_ — no hyphens, dots, or special chars.

store() {
  local path="$1" content="$2" tags="$3"
  local resp id
  resp=$(curl -s -X POST "$API/memories" \
    -H "X-API-Key: $KEY" \
    -H "Content-Type: application/json" \
    -d "{\"path\":\"$path\",\"content\":\"$content\",\"tags\":$tags,\"embedding_model\":\"text-embedding-3-small\"}")
  id=$(echo "$resp" | jq -r '.id // ""')
  if [ -z "$id" ] || [ "$id" = "null" ]; then
    warn "skip $path: $(echo "$resp" | jq -r '.error // "unknown"')"
    echo ""
    return 0
  fi
  echo "$id"
}

link() {
  local from="$1" to="$2" type="$3"
  if [ -z "$from" ] || [ "$from" = "null" ] || [ -z "$to" ] || [ "$to" = "null" ]; then
    return 0
  fi
  curl -s -X POST "$API/memories/links" \
    -H "X-API-Key: $KEY" \
    -H "Content-Type: application/json" \
    -d "{\"from_path\":\"memory.$from\",\"to_path\":\"memory.$to\",\"link_type\":\"$type\"}" \
    >/dev/null 2>&1 || true
}

OK_COUNT=0
ok() {
  OK_COUNT=$((OK_COUNT + 1))
  printf "  ${GREEN}OK${RESET}  %s\n" "$*"
}

# ── Scenario A: Production outage (8 nodes, causal chain + contradictions) ─
hdr "Scenario A — Production outage"

A1=$(store "root.a.crash" \
  "Production crashed at 03:15 UTC after config reload, 502 errors across all endpoints" \
  '["incident","crash"]')
info "A1 crash report    = $A1"

A2=$(store "root.a.stacktrace" \
  "SIGSEGV in config_parse at config.c line 142, null pointer dereference when parser_buffer is NULL" \
  '["incident","diagnosis"]')
info "A2 stacktrace      = $A2"

A3=$(store "root.a.fix_nullguard" \
  "Added null check before dereferencing parser_buffer, deployed as version 2_3_2 hotfix" \
  '["fix"]')
info "A3 null guard fix  = $A3"

A4=$(store "root.a.slowdown" \
  "After hotfix p99 latency jumped from 45ms to 320ms, the null guard branch defeats CPU branch prediction" \
  '["incident","performance","slowdown"]')
info "A4 slowdown        = $A4"

A5=$(store "root.a.fix_branch" \
  "Replaced null guard with __builtin_expect to hint compiler about non_null branch, p99 back to 48ms" \
  '["fix","performance"]')
info "A5 branch fix      = $A5"

A6=$(store "root.a.stable" \
  "Version 2_3_3 stable for 72 hours: p99 48ms, zero crashes, zero SLO violations, postmortem completed" \
  '["stable","postmortem"]')
info "A6 stable          = $A6"

A7=$(store "root.a.alt_rootcause" \
  "Alternative theory: crash was NOT a null pointer but a stack overflow from recursive config_validate" \
  '["theory","contradiction"]')
info "A7 alt theory      = $A7"

A8=$(store "root.a.rca_confirmed" \
  "Valgrind confirms null pointer at config.c line 142, stack overflow theory incorrect, recursion within limits" \
  '["rca","confirmed"]')
info "A8 RCA confirmed   = $A8"

link "$A1" "$A2" "causal"
link "$A2" "$A3" "causal"
link "$A3" "$A4" "causal"
link "$A4" "$A5" "causal"
link "$A5" "$A6" "causal"
link "$A3" "$A7" "contradicts"
link "$A7" "$A8" "contradicts"
link "$A2" "$A8" "supports"
link "$A1" "$A4" "temporal"
link "$A4" "$A8" "temporal"

ok "8 nodes, 10 edges: causal chain + contradictions + supports"

# ── Scenario B: Security incident (5 nodes) ─────────────────────────────────
hdr "Scenario B — Security data leak"

B1=$(store "root.b.leak" \
  "S3 bucket with debug logs accidentally made public, contains request payloads from last 72 hours" \
  '["security","leak"]')
info "B1 leak            = $B1"

B2=$(store "root.b.audit" \
  "CloudTrail shows devops_service_account ran PutBucketAcl at 02:47 UTC, ACL set to public_read" \
  '["security","audit"]')
info "B2 audit           = $B2"

B3=$(store "root.b.fix" \
  "Rotated all API keys, disabled devops_service_account, enabled S3 block_public_access at account level" \
  '["security","fix"]')
info "B3 fix             = $B3"

B4=$(store "root.b.postmortem" \
  "Root cause: Terraform module v3_6 defaulted acl public_read for log buckets, fixed in tf_modules v3_7" \
  '["security","postmortem"]')
info "B4 postmortem      = $B4"

B5=$(store "root.b.monitoring" \
  "Added AWS Config rule to detect public S3 buckets within 60 seconds, integrated with PagerDuty" \
  '["security","monitoring","prevention"]')
info "B5 monitoring      = $B5"

link "$B1" "$B2" "causal"
link "$B2" "$B3" "causal"
link "$B3" "$B4" "causal"
link "$B3" "$B5" "related"
link "$B4" "$B5" "supports"
link "$A1" "$B1" "temporal"

ok "5 nodes, 6 edges + cross-scenario temporal link"

# ── Scenario C: Feature development (5 nodes, diamond pattern) ──────────────
hdr "Scenario C — Cache feature"

C1=$(store "root.c.cache_design" \
  "Design: Redis cache layer for config lookups with 5min TTL, projected 60 percent DB load reduction" \
  '["feature","design","cache"]')
info "C1 design          = $C1"

C2=$(store "root.c.cache_impl" \
  "Implemented write_through cache in config_resolve, deployed to staging, all integration tests green" \
  '["feature","implementation","cache"]')
info "C2 impl            = $C2"

C3=$(store "root.c.cache_bug" \
  "Cache serves stale values for up to 5min after config reload, hot_reload is effectively broken" \
  '["bug","cache","stale"]')
info "C3 bug             = $C3"

C4=$(store "root.c.cache_fix" \
  "Added cache invalidation via Redis pubsub on config reload, TTL cleared within 50ms of SIGHUP" \
  '["fix","cache"]')
info "C4 fix             = $C4"

C5=$(store "root.c.cache_metrics" \
  "Added dashboards: cache hit rate 94_3 percent, invalidation latency p99 48ms, stale_read count zero" \
  '["observability","metrics","cache"]')
info "C5 metrics         = $C5"

link "$C1" "$C2" "causal"
link "$C2" "$C3" "causal"
link "$C3" "$C4" "causal"
link "$C2" "$C4" "causal"
link "$C4" "$C5" "causal"
link "$C1" "$A4" "temporal"
link "$C4" "$A5" "supports"
link "$C3" "$A3" "related"

ok "5 nodes, 8 edges: diamond pattern + cross-scenario"

# ── Scenario D: Rollback (3 nodes) ──────────────────────────────────────────
hdr "Scenario D — Rollback"

D1=$(store "root.d.rollback" \
  "Version 2_3_2 rolled back at 06:30 UTC, DNS timeouts increased to 15 percent of all requests" \
  '["rollback","incident"]')
info "D1 rollback        = $D1"

D2=$(store "root.d.hotfix" \
  "Version 2_3_3 hotfix deployed: reverts branch prediction change, adds proper mutex guard in config_parse" \
  '["rollback","hotfix"]')
info "D2 hotfix          = $D2"

D3=$(store "root.d.stable" \
  "Version 2_3_3 running stable for 48 hours: p99 52ms, zero crashes, DNS timeouts back to baseline 0_02 percent" \
  '["stable","postmortem"]')
info "D3 stable          = $D3"

link "$D1" "$D2" "causal"
link "$D2" "$D3" "causal"
link "$A3" "$D1" "temporal"
link "$A5" "$D1" "contradicts"

ok "3 nodes, 4 edges + contradictions"

# ── Scenario E: Synthesis hub (4 nodes) ─────────────────────────────────────
hdr "Scenario E — Synthesis hub"

E1=$(store "root.e.master_postmortem" \
  "Master postmortem: March outage had 3 contributing factors: null pointer bug, S3 bucket leak, missing deadman switch" \
  '["postmortem","synthesis"]')
info "E1 master          = $E1"

E2=$(store "root.e.action_items" \
  "Action items: mandatory valgrind in CI, AWS Config for S3, deadman switch on all services, runbook review" \
  '["action","process"]')
info "E2 actions         = $E2"

E3=$(store "root.e.ci_improvement" \
  "Added valgrind leak_check full to CI pipeline, blocks merge if memory errors detected, first run found 3 leaks" \
  '["ci","improvement","prevention"]')
info "E3 CI improvement  = $E3"

E4=$(store "root.e.quarterly_review" \
  "Q1 review: incident count down 60 percent, mean time to detect down from 45min to 3min, team velocity recovered" \
  '["review","metrics","improvement"]')
info "E4 quarterly review = $E4"

link "$A6" "$E1" "supports"
link "$B4" "$E1" "supports"
link "$D3" "$E1" "supports"
link "$E1" "$E2" "causal"
link "$E2" "$E3" "causal"
link "$E3" "$E4" "causal"
link "$A5" "$E3" "supports"
link "$B5" "$E3" "related"
link "$C5" "$E4" "causal"

ok "4 nodes, 9 edges: synthesis hub with 3 incoming supports"

# ── Summary ─────────────────────────────────────────────────────────────────
sleep 2

TOTAL_AGE=$(curl -s -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"query":"MATCH (n:Memory) RETURN n.id"}' \
  "$API/graph/cypher" | jq -r '.rows | length // 0')

hdr "Seed complete"
echo ""
echo "  Vertices in AGE graph: $TOTAL_AGE"
echo "  Total scenarios:       A=outage B=security C=cache D=rollback E=synthesis"
echo ""
echo "  Open: ${API_URL:-http://localhost:8000}/v1/graph/ui"
echo "  Key:  $KEY"
echo ""
echo "  Try in the UI:"
echo "    Memory ID $A1, depth 5 — full incident graph"
echo "    Memory ID $E1, depth 3 — synthesis hub"
echo "    Chain from=$A1 to=$A6 — 5-hop causal path"
echo "    Chain from=$C1 to=$C5 — diamond pattern"
echo "    Memory ID $B1, depth 3 — security scenario"
