#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://127.0.0.1:8000/v1/health}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-45}"
SLEEP_SECS="${SLEEP_SECS:-2}"

for i in $(seq 1 "$MAX_ATTEMPTS"); do
  if curl -sf "$API_URL" >/dev/null; then
    echo "API healthy ($API_URL)"
    exit 0
  fi
  sleep "$SLEEP_SECS"
done

echo "API did not become healthy ($API_URL)" >&2
exit 1
