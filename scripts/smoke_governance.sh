#!/usr/bin/env bash
# Smoke test for the governance / DX surface, driven through the `pcmi` CLI:
# seed + retrieve + time travel, tail (SSE), namespace retention policies,
# GDPR erasure, tamper-evident audit (verify, export, offline verify-export),
# and usage metering. Needs a running API (+ worker for embeddings).
#
#   PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123 scripts/smoke_governance.sh
#
# Optional:
#   AUDIT_KEY_FILE=path  file with the API's AUDIT_EXPORT_SIGNING_KEY → require a signed export
#   EXPECT_RERANK=1      API runs with RERANK_ENABLED=true and an LLM → require reranked=true
#   EXPECT_USAGE=1       API/worker call an embedding/LLM provider → require metered tokens
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
export PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"
API="$PCMI_BASE_URL"
hdr=(-H "Content-Type: application/json" -H "X-API-Key: ${PCMI_API_KEY}")

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"; [[ -n "${TAIL_PID:-}" ]] && kill "$TAIL_PID" 2>/dev/null || true' EXIT
PCMI="$WORK/pcmi"
go build -o "$PCMI" ./cmd/pcmi

PREFIX="root.smoke_gov_$(date +%s)"
pass() { echo "  ✓ $*"; }
die() { echo "  ✗ $*" >&2; exit 1; }

echo "== version =="
"$PCMI" version | grep "pcmi CLI" >/dev/null || die "cli + server reachable"
pass "cli + server reachable"

echo "== tail (SSE) =="
"$PCMI" --json tail -n 1 --types memory.stored >"$WORK/tail.log" 2>/dev/null &
TAIL_PID=$!
sleep 1

echo "== seed + retrieve + time travel =="
"$PCMI" seed --prefix "$PREFIX" | grep "stored 12 memories" >/dev/null || die "seeded demo corpus"
pass "seeded demo corpus"
"$PCMI" --json retrieve --prefix "$PREFIX" --query "latency or decision" --limit 5 >"$WORK/ret.json"
jq -e '.entries | length >= 1' "$WORK/ret.json" >/dev/null || die "retrieve returned $(jq '.entries|length' "$WORK/ret.json") entries"
pass "retrieve returned $(jq '.entries|length' "$WORK/ret.json") entries"
if [[ "${EXPECT_RERANK:-0}" == "1" ]]; then
  jq -e '.reranked == true' "$WORK/ret.json" >/dev/null || die "LLM reranked"
  pass "LLM reranked"
  "$PCMI" --json retrieve --prefix "$PREFIX" --query "latency or decision" --no-rerank | jq -e '(.reranked // false) == false' >/dev/null || die "rerank opt-out honoured"
  pass "rerank opt-out honoured"
fi
"$PCMI" get "$PREFIX.product.pricing.plan" --version 1 | grep "20 USD" >/dev/null || die "version 1 still readable"
pass "version 1 still readable"
"$PCMI" get "$PREFIX.product.pricing.plan" | grep "25 USD" >/dev/null || die "current version is v2"
pass "current version is v2"

wait "$TAIL_PID" 2>/dev/null || true
TAIL_PID=""
jq -e '(.type // .Type) == "memory.stored"' "$WORK/tail.log" >/dev/null || die "tail received memory.stored"
pass "tail received memory.stored"

echo "== retention policies =="
curl -sf -X PUT "$API/v1/retention-policies" "${hdr[@]}" \
  -d "{\"path_prefix\":\"$PREFIX.ops\",\"superseded_retention_days\":2555,\"description\":\"smoke\"}" | jq -e '.superseded_retention_days == 2555' >/dev/null || die "policy upserted"
  pass "policy upserted"
curl -sf "$API/v1/retention-policies" -H "X-API-Key: $PCMI_API_KEY" | jq -e --arg p "$PREFIX.ops" '[.policies[] | select(.path_prefix == $p)] | length == 1' >/dev/null || die "policy listed"
pass "policy listed"
code=$(curl -s -o /dev/null -w '%{http_code}' -X PUT "$API/v1/retention-policies" "${hdr[@]}" -d '{"path_prefix":"x"}')
[[ "$code" == "400" ]] || die "policy without a rule rejected (400)"
pass "policy without a rule rejected (400)"
curl -sf -X DELETE "$API/v1/retention-policies?path_prefix=$PREFIX.ops" -H "X-API-Key: $PCMI_API_KEY" | jq -e '.status == "deleted"' >/dev/null || die "policy deleted"
pass "policy deleted"

echo "== GDPR erasure =="
"$PCMI" --json erase "$PREFIX.customers.acme" --dry-run | jq -e '.dry_run == true and .counts.memory_versions == 2' >/dev/null || die "dry run counts 2 versions"
pass "dry run counts 2 versions"
"$PCMI" --json retrieve --prefix "$PREFIX.customers.acme" | jq -e '.entries | length == 2' >/dev/null || die "dry run deleted nothing"
pass "dry run deleted nothing"
if "$PCMI" erase "$PREFIX.customers.acme" >/dev/null 2>&1; then die "erase without --yes must refuse"; fi
pass "erase without --yes refused"
"$PCMI" --json erase "$PREFIX.customers.acme" --yes --reason "smoke DSR" | jq -e '.counts.memory_versions == 2 and (.erased_at | type) == "string"' >/dev/null || die "erased"
pass "erased"
"$PCMI" --json retrieve --prefix "$PREFIX.customers.acme" | jq -e '.entries | length == 0' >/dev/null || die "nothing left under the prefix"
pass "nothing left under the prefix"
"$PCMI" --json retrieve --prefix "$PREFIX.customers.globex" | jq -e '.entries | length == 1' >/dev/null || die "sibling prefix untouched"
pass "sibling prefix untouched"

echo "== tamper-evident audit =="
"$PCMI" audit verify | grep "audit chain intact" >/dev/null || die "server-side chain verification"
pass "server-side chain verification"
"$PCMI" audit export -o "$WORK/audit.jsonl" --limit 50000 2>/dev/null
tail -n 1 "$WORK/audit.jsonl" | jq -e '.type == "trailer" and .chain_valid == true' >/dev/null || die "sealed export downloaded"
pass "sealed export downloaded"
grep -q '"event_type":"gdpr_erase"' "$WORK/audit.jsonl" || die "gdpr_erase evidence is in the chain"
pass "gdpr_erase evidence is in the chain"
if [[ -n "${AUDIT_KEY_FILE:-}" ]]; then
  "$PCMI" audit verify-export "$WORK/audit.jsonl" --signing-key-file "$AUDIT_KEY_FILE" --require-signature >/dev/null || die "offline verification incl. HMAC signature"
  pass "offline verification incl. HMAC signature"
else
  "$PCMI" audit verify-export "$WORK/audit.jsonl" >/dev/null || die "offline verification"
  pass "offline verification"
fi
sed '1s/"status_code":[0-9]*/"status_code":999/' "$WORK/audit.jsonl" >"$WORK/tampered.jsonl"
if "$PCMI" audit verify-export "$WORK/tampered.jsonl" >/dev/null 2>&1; then die "tampered export verified"; fi
pass "tampered export rejected"

echo "== usage metering =="
"$PCMI" --json usage >"$WORK/usage.json"
jq -e '(.rows | type) == "array" and (.totals.requests | type) == "number"' "$WORK/usage.json" >/dev/null || die "usage report served"
pass "usage report served"
if [[ "${EXPECT_USAGE:-0}" == "1" ]]; then
  for _ in $(seq 1 20); do
    "$PCMI" --json usage >"$WORK/usage.json"
    jq -e '.totals.input_tokens > 0' "$WORK/usage.json" >/dev/null && break
    sleep 3
  done
  jq -e '.totals.input_tokens > 0' "$WORK/usage.json" >/dev/null || die "tokens metered: $(jq -c '[.rows[] | {operation, requests, input_tokens}]' "$WORK/usage.json")"
  pass "tokens metered: $(jq -c '[.rows[] | {operation, requests, input_tokens}]' "$WORK/usage.json")"
fi

echo "GOVERNANCE SMOKE: PASSED"
