#!/usr/bin/env bash
# SDK HTTP smoke (Python + TypeScript) — requires API on PCMI_BASE_URL.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
export PCMI_API_KEY="${PCMI_API_KEY:-testkey123}"

echo "SDK smoke against ${PCMI_BASE_URL}"

curl -sf "${PCMI_BASE_URL}/v1/health" >/dev/null || {
  echo "API not healthy at ${PCMI_BASE_URL}" >&2
  exit 1
}

echo "== Python SDK =="
python3 -m pip install -q -e "${ROOT}/sdk/python"
python3 "${ROOT}/sdk/python/smoke.py"
python3 "${ROOT}/sdk/python/admin_smoke.py"

echo "== TypeScript SDK =="
(
  cd "${ROOT}/sdk/typescript"
  npm ci --silent
  npm run smoke
  npm run admin-smoke
)

echo "SDK smoke OK"
