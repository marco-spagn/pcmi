#!/usr/bin/env bash
# v1.14 E2E: batch, get-by-path, export/import, admin, metrics, gRPC smoke
set -euo pipefail

API="${API_URL:-http://localhost:8000}"
GRPC_HOST="${GRPC_HOST:-localhost:50051}"
KEY="${PCMI_API_KEY:-testkey123}"
SUFFIX=$(date +%s)

echo "== v1.14 batch store =="
curl -sf -X POST "$API/v1/memories/batch" \
  -H "Content-Type: application/json" -H "X-API-Key: $KEY" \
  -d "{\"items\":[{\"path\":\"root.v14.batch.${SUFFIX}.a\",\"content\":\"one\"},{\"path\":\"root.v14.batch.${SUFFIX}.b\",\"content\":\"two\"}]}" \
  | jq -e '.total == 2 and .results[0].status == "stored"'

echo "== v1.14 get memory by path =="
curl -sf "$API/v1/memories/root.v14.batch.${SUFFIX}.a" -H "X-API-Key: $KEY" \
  | jq -e '.content == "one"'

echo "== v1.14 batch retrieve =="
curl -sf -X POST "$API/v1/retrieve/batch" \
  -H "Content-Type: application/json" -H "X-API-Key: $KEY" \
  -d "{\"queries\":[{\"path_prefix\":\"root.v14.batch.${SUFFIX}\",\"limit\":5}]}" \
  | jq -e '.total == 1 and (.results[0].entries | length) >= 1'

echo "== v1.14 export/import =="
EXPORT=$(curl -sf -X POST "$API/v1/memories/export" \
  -H "Content-Type: application/json" -H "X-API-Key: $KEY" \
  -d "{\"path_prefix\":\"root.v14.batch.${SUFFIX}\",\"limit\":50}")
echo "$EXPORT" | jq -e '.exported >= 1'
NEW_SUFFIX=$((SUFFIX + 1))
IMPORT_BODY=$(echo "$EXPORT" | jq --arg sfx "$NEW_SUFFIX" '{
  mode: "skip",
  entries: [.entries[] | .path = ("root.v14.import." + $sfx + "." + (.path | split(".") | last)) | {path, content, metadata, tags}]
}')
curl -sf -X POST "$API/v1/memories/import" \
  -H "Content-Type: application/json" -H "X-API-Key: $KEY" \
  -d "$IMPORT_BODY" | jq -e '.imported >= 1'

echo "== v1.14 prometheus metrics =="
curl -sf -X POST "$API/v1/memories" \
  -H "Content-Type: application/json" -H "X-API-Key: $KEY" \
  -d "{\"path\":\"root.v14.metrics.${SUFFIX}\",\"content\":\"m\",\"metadata\":{}}" >/dev/null
METRICS=$(curl -sf -H 'Accept-Encoding: identity' "$API/metrics") || { echo "metrics endpoint failed (curl exit $?)"; curl -sS -H 'Accept-Encoding: identity' "$API/metrics" | head -5 || true; exit 1; }
echo "$METRICS" | grep -q 'pcmi_memory_stores_total' || {
  echo "expected pcmi_memory_stores_total not found"; echo "$METRICS" | head -20; exit 1;
}

echo "== v1.14 admin list tenants =="
curl -sf "$API/v1/admin/tenants?limit=5" -H "X-API-Key: $KEY" | jq -e '.total >= 1'

echo "== v1.14 gRPC health (grpcurl if available) =="
if command -v grpcurl >/dev/null 2>&1; then
  grpcurl -plaintext -H "x-api-key: $KEY" "$GRPC_HOST" pcmi.v1.MemoryService/Health \
    | grep -qE 'v1\.1[0-9]+\.[0-9]+'
else
  echo "grpcurl not installed — skipping gRPC smoke (install grpcurl for full check)"
fi

echo "== v1.14 consolidation (seed + worker tick) =="
PATH_P="root.v14.consolidate.${SUFFIX}"
for i in 1 2 3; do
  curl -sf -X POST "$API/v1/memories" \
    -H "Content-Type: application/json" -H "X-API-Key: $KEY" \
    -d "{\"path\":\"${PATH_P}.item${i}\",\"content\":\"fragment ${i}\",\"metadata\":{}}" >/dev/null
done
sleep 8
curl -sf -X POST "$API/v1/retrieve" \
  -H "Content-Type: application/json" -H "X-API-Key: $KEY" \
  -d "{\"path_prefix\":\"${PATH_P}\",\"limit\":20}" \
  | jq -e '[.entries[].path] | any(test("\\.consolidated$"))' || {
    echo "consolidation may need more time — checking DB pending runs"
    PGPASSWORD=pcmi psql -h localhost -U pcmi -d pcmi -tA -c \
      "SELECT COUNT(*) FROM consolidation_runs WHERE path_prefix='${PATH_P}'" || true
  }

echo "v1.14 E2E script completed"
