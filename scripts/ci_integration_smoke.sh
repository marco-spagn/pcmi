#!/usr/bin/env bash
# Integration smoke: same coverage as former multi-step CI job, single entrypoint.
# Run from repository root with API + worker + Postgres + Redis already up.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# PostgreSQL host for psql sub-commands (CI and act use 127.0.0.1 to avoid ::1 surprises).
PGHOST="${PGHOST:-localhost}"

API="${API:-http://localhost:8000}"
KEY="${PCMI_API_KEY:-testkey123}"
export PCMI_EXPECT_VERSION="${PCMI_EXPECT_VERSION:-v1.47.0}"
VER="${EXPECT_API_VERSION:-v1.47.0}"
hdr=(-H "Content-Type: application/json" -H "X-API-Key: ${KEY}")

echo "== Readiness /v1/ready =="
curl -sf "${API}/v1/ready" | jq -e '.status == "ready" and .database_ok == true and .redis_ok == true'

echo "== Store and retrieve (no OpenAI) =="
SUFFIX=$(date +%s)
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"root.ci.${SUFFIX}\",\"content\":\"CI smoke test\",\"metadata\":{},\"embedding_model\":\"text-embedding-3-small\",\"tags\":[\"ci\"]}" | jq -e '.id'
curl -sf -X POST "${API}/v1/retrieve" "${hdr[@]}" \
  -d '{"path_prefix":"root.ci","limit":10}' | jq -e '.entries | length >= 1'
curl -sf "${API}/v1/distilled?path_prefix=root.ci&limit=5" -H "X-API-Key: ${KEY}" | jq -e '(.entries | type) == "array"'

echo "== Temporal append-only =="
PATH_V="root.ci.temporal.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_V}\",\"content\":\"version-one\",\"metadata\":{}}" | jq -e '.version == 1'
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_V}\",\"content\":\"version-two\",\"metadata\":{}}" | jq -e '.version == 2 and (.superseded_id | type) == "number"'
CONTENT=$(curl -sf -X POST "${API}/v1/retrieve" "${hdr[@]}" -d "{\"path_prefix\":\"${PATH_V}\",\"limit\":5}" | jq -r '.entries[0].content')
test "$CONTENT" = "version-two"

echo "== SSE memory.stored =="
EVENT_LOG=$(mktemp)
curl -sN --max-time 12 "${API}/v1/events?types=memory.stored" -H "X-API-Key: ${KEY}" -H "Accept: text/event-stream" >"$EVENT_LOG" &
STREAM_PID=$!
sleep 2
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"root.ci.events.${SUFFIX}\",\"content\":\"event smoke\",\"metadata\":{}}" | jq -e '.id'
sleep 4
kill "$STREAM_PID" 2>/dev/null || true
wait "$STREAM_PID" 2>/dev/null || true
grep -q 'memory.stored' "$EVENT_LOG"

echo "== Rollback =="
PATH_R="root.ci.rollback.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_R}\",\"content\":\"alpha\",\"metadata\":{}}" | jq -e '.version == 1'
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_R}\",\"content\":\"beta\",\"metadata\":{}}" | jq -e '.version == 2'
curl -sf -X POST "${API}/v1/memories/rollback" "${hdr[@]}" -d "{\"path\":\"${PATH_R}\",\"version\":1}" | jq -e '.restored_from_version == 1 and .version == 3'
CONTENT=$(curl -sf -X POST "${API}/v1/retrieve" "${hdr[@]}" -d "{\"path_prefix\":\"${PATH_R}\",\"limit\":5}" | jq -r '.entries[0].content')
test "$CONTENT" = "alpha"

echo "== Audit + event ingest + history =="
curl -sf "${API}/v1/audit?limit=10" -H "X-API-Key: ${KEY}" | jq -e '(.entries | type) == "array" and (.total | type) == "number" and (.total >= 1)'
curl -sf -X POST "${API}/v1/events" "${hdr[@]}" -d "{\"event_type\":\"agent.step.completed\",\"payload\":{\"step\":\"ci.${SUFFIX}\"}}" | jq -e '.status == "ingested" and (.id | type) == "string"'
PATH_H="root.ci.history.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_H}\",\"content\":\"v1\",\"metadata\":{}}" | jq -e '.version == 1'
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_H}\",\"content\":\"v2\",\"metadata\":{}}" | jq -e '.version == 2'
curl -sf "${API}/v1/memories/history?path=${PATH_H}" -H "X-API-Key: ${KEY}" | jq -e '(.entries | length) == 2 and .entries[0].version == 2'

echo "== Encrypted at rest =="
PATH_E="root.ci.encrypt.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_E}\",\"content\":\"top-secret-${SUFFIX}\",\"metadata\":{\"sensitive\":true},\"embedding_model\":\"unspecified\"}" | jq -e '.id'
CONTENT=$(curl -sf -X POST "${API}/v1/retrieve" "${hdr[@]}" -d "{\"path_prefix\":\"${PATH_E}\",\"limit\":5}" | jq -r '.entries[0].content')
test "$CONTENT" = "top-secret-${SUFFIX}"
STORED=$(PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -tA -c "SELECT content FROM memory_entries WHERE path='${PATH_E}'::ltree AND valid_to IS NULL LIMIT 1")
echo "$STORED" | grep -q '^enc:v1:' || {
  echo "expected encrypted at rest"
  exit 1
}

echo "== Webhook delivery =="
python3 "${ROOT}/scripts/ci_smoke_webhook_receiver.py" 9876 &
WH_PID=$!
sleep 1
curl -sf -X POST "${API}/v1/webhooks" "${hdr[@]}" -d '{"url":"http://127.0.0.1:9876/hook","event_types":["memory.stored"],"secret":"ci-secret"}' | jq -e '.status == "registered"'
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"root.ci.webhook.${SUFFIX}\",\"content\":\"wh\",\"metadata\":{}}" | jq -e '.id'
sleep 8
kill "$WH_PID" 2>/dev/null || true
wait "$WH_PID" 2>/dev/null || true
test -f /tmp/webhook_hits.json
python3 -c "import json; hits=json.load(open('/tmp/webhook_hits.json')); assert hits; assert 'memory.stored' in hits[0]"

echo "== Embedding migrate =="
PATH_P="root.ci.migrate.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_P}\",\"content\":\"migrate-me\",\"metadata\":{},\"embedding_model\":\"text-embedding-3-small\"}" | jq -e '.id'
curl -sf -X POST "${API}/v1/embeddings/migrate" "${hdr[@]}" -d "{\"path_prefix\":\"${PATH_P}\",\"target_model\":\"text-embedding-3-large\"}" | jq -e '.marked_count >= 1'
PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -tA -c "SELECT embedding IS NULL AND embedding_model='text-embedding-3-large' FROM memory_entries WHERE path='${PATH_P}'::ltree AND valid_to IS NULL" | tr -d '[:space:]' | grep -qx 't'

echo "== Distilled versioning (SQL seed) =="
PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -v ON_ERROR_STOP=1 <<SQL
INSERT INTO distilled_knowledge (tenant_id, path, summary, insights, confidence_score, source_entry_ids, version)
VALUES ('00000000-0000-0000-0000-000000000000', 'root.ci.distilled.version', 'v1 summary', '[]', 0.9, ARRAY[1]::bigint[], 1);
INSERT INTO distilled_knowledge (tenant_id, path, summary, insights, confidence_score, source_entry_ids, version)
VALUES ('00000000-0000-0000-0000-000000000000', 'root.ci.distilled.version', 'v2 summary', '[]', 0.9, ARRAY[2]::bigint[], 2);
SQL
MAX_VER=$(curl -sf "${API}/v1/distilled?path_prefix=root.ci.distilled&limit=10" -H "X-API-Key: ${KEY}" | jq '[.entries[].version] | max')
test "$MAX_VER" -ge 2

echo "== Pruning function =="
PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -v ON_ERROR_STOP=1 <<SQL
INSERT INTO memory_entries (tenant_id, path, content, metadata, embedding_model, version, valid_from, valid_to)
VALUES ('00000000-0000-0000-0000-000000000000', 'root.ci.prune.old', 'stale', '{}', 'unspecified', 1,
        NOW() - interval '60 days', NOW() - interval '45 days');
SQL
PRUNED=$(PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -tA -c "SELECT prune_superseded_memories(30)")
test "$PRUNED" -ge 1
COUNT=$(PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -tA -c "SELECT COUNT(*) FROM memory_entries WHERE path='root.ci.prune.old'::ltree")
test "$COUNT" -eq 0

echo "== Event schemas + invalid payload + summarize + /v1/health version =="
curl -sf "${API}/v1/events/schemas" -H "X-API-Key: ${KEY}" | jq -e '(.schemas | length) >= 5 and (.total >= 5)'
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API}/v1/events" "${hdr[@]}" -d '{"event_type":"agent.step.failed","payload":{}}')
test "$code" = "400"
PATH_SUM="root.ci.summarize.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_SUM}\",\"content\":\"Summarize me: alpha beta gamma\",\"metadata\":{}}" | jq -e '.id'
curl -sf -X POST "${API}/v1/memories/summarize" "${hdr[@]}" -d "{\"path_prefix\":\"${PATH_SUM}\",\"limit\":5,\"style\":\"brief\"}" | jq -e '.method == "extractive" and (.summary | length) > 0 and (.total >= 1)'
curl -sf "${API}/v1/health" -H "X-API-Key: ${KEY}" | jq -e --arg v "$VER" '.pool.total_conns >= 1 and .version == $v'

echo "== v1.14 batch / get / export / metrics / admin / gRPC =="
PATH_G="root.ci.get.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_G}\",\"content\":\"single\",\"metadata\":{}}"
curl -sf "${API}/v1/memories/${PATH_G}" -H "X-API-Key: ${KEY}" | jq -e '.content == "single"'
curl -sf -X POST "${API}/v1/memories/batch" "${hdr[@]}" -d "{\"items\":[{\"path\":\"root.ci.batch.${SUFFIX}.x\",\"content\":\"bx\"}]}" | jq -e '.results[0].status == "stored"'
curl -sf -X POST "${API}/v1/retrieve/batch" "${hdr[@]}" -d "{\"queries\":[{\"path_prefix\":\"root.ci.batch.${SUFFIX}\",\"limit\":5}]}" | jq -e '.results[0].total >= 1'
PATH_X="root.ci.export.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_X}\",\"content\":\"migrate\",\"metadata\":{}}"
curl -sf -X POST "${API}/v1/memories/export" "${hdr[@]}" -d "{\"path_prefix\":\"${PATH_X}\",\"limit\":10}" | jq -e '.exported >= 1'
curl -sf -X POST "${API}/v1/memories/import" "${hdr[@]}" -d "{\"mode\":\"skip\",\"entries\":[{\"path\":\"${PATH_X}\",\"content\":\"dup\",\"metadata\":{}}]}" | jq -e '.skipped >= 1'
curl -sf "${API}/health" >/dev/null
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d '{"path":"root.ci.metrics","content":"m","metadata":{}}' >/dev/null
curl -sf -H 'Accept-Encoding: identity' "${API}/metrics" | grep -qE 'pcmi_memory_(stores|retrieves)_total'
WORKER_HTTP="${WORKER_HTTP:-http://localhost:8081}"
curl -sf -H 'Accept-Encoding: identity' "${WORKER_HTTP}/metrics" | grep -qE 'pcmi_worker_redis_events_total'
curl -sf "${API}/v1/admin/tenants?limit=5" -H "X-API-Key: ${KEY}" | jq -e '.total >= 1'
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 2>/dev/null || true
GRPC_HOST="${GRPC_HOST:-localhost:50051}" GRPC_TEST_API_KEY="${KEY}" go run ./scripts/grpc_health_smoke.go

echo "== v1.15 stats / tags / links / refine / lineage / TTL =="
curl -sf "${API}/v1/stats" -H "X-API-Key: ${KEY}" | jq -e '.active_memories >= 1'
PATH_T="root.ci.tags.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_T}\",\"content\":\"tagged\",\"metadata\":{},\"tags\":[\"soc\",\"alert\"]}"
curl -sf -X POST "${API}/v1/retrieve" "${hdr[@]}" -d "{\"path_prefix\":\"root.ci.tags\",\"tags\":[\"soc\"],\"tags_match\":\"any\",\"limit\":20}" | jq -e --arg p "$PATH_T" '[.entries[].path] | index($p) != null'
FROM="root.ci.link.${SUFFIX}.a"
TO="root.ci.link.${SUFFIX}.b"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${FROM}\",\"content\":\"a\",\"metadata\":{}}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${TO}\",\"content\":\"b\",\"metadata\":{}}"
curl -sf -X POST "${API}/v1/memories/links" "${hdr[@]}" -d "{\"from_path\":\"${FROM}\",\"to_path\":\"${TO}\",\"link_type\":\"related\"}" | jq -e '.id'
PREFIX="root.ci.refine.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories/refine" "${hdr[@]}" -d "{\"path_prefix\":\"${PREFIX}\"}" | jq -e '.status == "queued"'
PATH_L="root.ci.lineage.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_L}\",\"content\":\"one\",\"metadata\":{}}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_L}\",\"content\":\"two\",\"metadata\":{}}"
curl -sf "${API}/v1/lineage/memory?path=${PATH_L}" -H "X-API-Key: ${KEY}" | jq -e '(.versions | length) == 2'
PATH_TTL="root.ci.ttl.${SUFFIX}"
# macOS (BSD) date: -v+3S — Linux (GNU) date: -d '+3 seconds'
EXPIRES=$(date -u -v+3S '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d '+3 seconds' '+%Y-%m-%dT%H:%M:%SZ')
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_TTL}\",\"content\":\"ttl\",\"metadata\":{},\"expires_at\":\"${EXPIRES}\"}"
sleep 4
PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -tA -c "SELECT expire_memory_entries()" >/dev/null
PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -tA -c "SELECT COUNT(*) FROM memory_entries WHERE path='${PATH_TTL}'::ltree AND valid_to IS NULL" | tr -d '[:space:]' | grep -qx '0'

echo "== Consolidation =="
PREFIX2="root.ci.consolidate.${SUFFIX}"
for i in 1 2 3; do
  curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PREFIX2}.n${i}\",\"content\":\"part ${i}\",\"metadata\":{}}" >/dev/null
done
sleep 10
RUNS=$(PGPASSWORD=pcmi psql -h "$PGHOST" -U pcmi -d pcmi -tA -c "SELECT COUNT(*) FROM consolidation_runs WHERE path_prefix='${PREFIX2}'" | tr -d '[:space:]')
test "${RUNS:-0}" -ge 1

echo "== Webhook dead-letter =="
curl -sf -X POST "${API}/v1/webhooks" "${hdr[@]}" -d '{"url":"http://127.0.0.1:9/dead","event_types":["memory.stored"],"secret":""}' | jq -e '.status == "registered"'
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"root.ci.dlq.${SUFFIX}\",\"content\":\"dlq\",\"metadata\":{}}" | jq -e '.id'
for i in $(seq 1 30); do
  n=$(curl -sf "${API}/v1/webhooks/dead-letter?limit=10" -H "X-API-Key: ${KEY}" | jq '.entries | length')
  if [ "${n:-0}" -ge 1 ]; then
    echo "dead-letter ok"
    break
  fi
  sleep 2
  if [ "$i" -eq 30 ]; then
    echo "no dead-letter"
    exit 1
  fi
done

echo "== Cross-agent scope =="
AGENT="00000000-0000-0000-0000-000000000001"
PATH_A="root.ci.scope.${SUFFIX}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_A}\",\"content\":\"agent-one\",\"metadata\":{},\"source_agent_id\":\"${AGENT}\",\"embedding_space\":\"experimental\"}"
curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"root.ci.scope.other.${SUFFIX}\",\"content\":\"agent-two\",\"metadata\":{}}"
COUNT=$(curl -sf -X POST "${API}/v1/retrieve" "${hdr[@]}" -d "{\"path_prefix\":\"root.ci.scope\",\"source_agent_id\":\"${AGENT}\",\"embedding_space\":\"experimental\",\"limit\":20}" | jq '.entries | length')
test "$COUNT" -ge 1

echo "== memory compaction API =="
PATH_C="root.ci.compact.${SUFFIX}"
for _v in 1 2 3 4 5 6; do
  curl -sf -X POST "${API}/v1/memories" "${hdr[@]}" -d "{\"path\":\"${PATH_C}\",\"content\":\"body\",\"metadata\":{}}" >/dev/null
done
HIST=$(curl -sf "${API}/v1/memories/history?path=${PATH_C}" -H "X-API-Key: ${KEY}" | jq '.entries | length')
test "$HIST" -eq 6
curl -sf -X POST "${API}/v1/memories/compact" "${hdr[@]}" -d "{\"path\":\"${PATH_C}\",\"keep_superseded\":2}" | jq -e '.deleted_count >= 1'
HIST2=$(curl -sf "${API}/v1/memories/history?path=${PATH_C}" -H "X-API-Key: ${KEY}" | jq '.entries | length')
test "$HIST2" -eq 3

echo "== All integration smoke checks passed =="
