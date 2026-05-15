#!/usr/bin/env bash
# CI E2E: v1.15 refine queue, tag filters, links, stats, TTL expiry.
set -euo pipefail

API_URL="${API_URL:-http://localhost:8000}"
API_KEY="${PCMI_API_KEY:-testkey123}"
SUFFIX="${SUFFIX:-$(date +%s)}"

hdr=(-H "Content-Type: application/json" -H "X-API-Key: ${API_KEY}")

echo "== v1.15 stats =="
curl -sf "${API_URL}/v1/stats" -H "X-API-Key: ${API_KEY}" \
  | jq -e '.active_memories >= 0 and .links_count >= 0'

PREFIX="root.ci.v15.${SUFFIX}"
TAG_A="${PREFIX}.tagged"

echo "== v1.15 tag filter =="
curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"${TAG_A}\",\"content\":\"tagged\",\"metadata\":{},\"tags\":[\"alpha\",\"beta\"]}"
curl -sf -X POST "${API_URL}/v1/retrieve" "${hdr[@]}" \
  -d "{\"path_prefix\":\"${PREFIX}\",\"tags\":[\"alpha\"],\"tags_match\":\"any\",\"limit\":10}" \
  | jq -e '[.entries[].path] | index("'"${TAG_A}"'") != null'

echo "== v1.15 memory links =="
LINK_FROM="${PREFIX}.from"
LINK_TO="${PREFIX}.to"
curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"${LINK_FROM}\",\"content\":\"from\",\"metadata\":{}}"
curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"${LINK_TO}\",\"content\":\"to\",\"metadata\":{}}"
curl -sf -X POST "${API_URL}/v1/memories/links" "${hdr[@]}" \
  -d "{\"from_path\":\"${LINK_FROM}\",\"to_path\":\"${LINK_TO}\",\"link_type\":\"supports\"}" \
  | jq -e '.from_path == "'"${LINK_FROM}"'"'
curl -sf "${API_URL}/v1/memories/links?from_path=${LINK_FROM}" -H "X-API-Key: ${API_KEY}" \
  | jq -e '.total >= 1'

echo "== v1.15 lineage =="
curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"${PREFIX}.lineage\",\"content\":\"v1\",\"metadata\":{}}" | jq -e '.version == 1'
curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"${PREFIX}.lineage\",\"content\":\"v2\",\"metadata\":{}}" | jq -e '.version == 2'
curl -sf "${API_URL}/v1/lineage/memory?path=${PREFIX}.lineage" -H "X-API-Key: ${API_KEY}" \
  | jq -e '(.versions | length) == 2'

echo "== v1.15 refine queue =="
curl -sf -X POST "${API_URL}/v1/memories/refine" "${hdr[@]}" \
  -d "{\"path_prefix\":\"${PREFIX}\"}" \
  | jq -e '.status == "queued" and .path_prefix == "'"${PREFIX}"'"'

echo "== v1.15 TTL expiry =="
TTL_PATH="${PREFIX}.ttl"
EXPIRES=$(date -u -v+2S '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -d '+2 seconds' '+%Y-%m-%dT%H:%M:%SZ')
curl -sf -X POST "${API_URL}/v1/memories" "${hdr[@]}" \
  -d "{\"path\":\"${TTL_PATH}\",\"content\":\"short-lived\",\"metadata\":{},\"expires_at\":\"${EXPIRES}\"}" \
  | jq -e '.id'
sleep 5
COUNT=$(curl -sf -X POST "${API_URL}/v1/retrieve" "${hdr[@]}" \
  -d "{\"path_prefix\":\"${TTL_PATH}\",\"limit\":5}" | jq '.total')
test "${COUNT:-1}" -eq 0

echo "v1.15 E2E OK"
