#!/usr/bin/env bash
# E2E smoke: CTI dataset + entity registry evolution + hybrid retrieval demos.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
CTI="${ROOT}/examples/full-cti-dataset"
export PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
export PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"

cd "$ROOT"

echo "→ validate CTI JSON"
python3 "${CTI}/validate.py"

echo "→ build operational STIX (cached bundles OK)"
make -s cti-stix-build

echo "→ API ready"
curl -sf "${PCMI_BASE_URL}/v1/ready" >/dev/null

echo "→ graph + retrieve smoke"
curl -sf -H "X-API-Key: ${PCMI_API_KEY}" "${PCMI_BASE_URL}/v1/graph/health" | jq -e '.available == true' >/dev/null
curl -sf -H "X-API-Key: ${PCMI_API_KEY}" -H "Content-Type: application/json" \
  -d '{"path_prefix":"root.cti","limit":3}' "${PCMI_BASE_URL}/v1/retrieve" | jq -e '(.entries|length) >= 3' >/dev/null

echo "→ registry detail (keys with spaces)"
curl -sf -H "X-API-Key: ${PCMI_API_KEY}" \
  "${PCMI_BASE_URL}/v1/entities/registry/ThreatActor/Sapphire%20Sleet" | jq -e '.snapshot_n >= 1' >/dev/null

echo "→ cross-vendor retrieval demo"
python3 "${CTI}/demo_cross_vendor_correlation.py"

echo "→ evolution + retrieval narrative demo"
python3 "${CTI}/demo_entity_evolution_retrieval.py" --skip-wait-embed

echo "→ graph UI tour (12 steps embedded)"
curl -sf "${PCMI_BASE_URL}/v1/graph/ui?demo=cti" | grep -q 'Passo 12 / 12'

echo "→ resolve tour IDs"
python3 "${CTI}/resolve_demo_ids.py" --json | jq -e '.ms_sapphire != null' >/dev/null

echo "✓ CTI evolution + retrieval demo smoke passed"
