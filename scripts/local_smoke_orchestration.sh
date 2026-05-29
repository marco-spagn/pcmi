#!/usr/bin/env bash
# Local smoke: store → immediate GET (read-replica lag if DATABASE_READ_URL is set)
# and optional Temporal example (examples/temporal) against a running PCMI API.
#
# Usage:
#   ./scripts/local_smoke_orchestration.sh replica
#   ./scripts/local_smoke_orchestration.sh temporal
#   ./scripts/local_smoke_orchestration.sh all
#
# Env:
#   PCMI_BASE_URL   (default http://localhost:8000)
#   PCMI_API_KEY    (default testkey123)
#   TEMPORAL_ADDRESS (default localhost:7233)
#   START_TEMPORAL_DEV=1  try to run `temporal server start-dev` in background (requires Temporal CLI)
#   SKIP_TEMPORAL=1       with `all`, run only replica
#   SKIP_TEMPORAL_VENV=1  use system python3 (must have temporalio + httpx); skips venv + pip

set -euo pipefail

BASE="${PCMI_BASE_URL:-http://localhost:8000}"
BASE="${BASE%/}"
KEY="${PCMI_API_KEY:-testkey123}"
TEMPORAL_ADDR="${TEMPORAL_ADDRESS:-localhost:7233}"
TEMPORAL_HOST="${TEMPORAL_ADDR%%:*}"
TEMPORAL_PORT="${TEMPORAL_ADDR##*:}"
[[ "$TEMPORAL_PORT" == "$TEMPORAL_ADDR" ]] && TEMPORAL_PORT="7233"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

usage() {
  echo "Usage: $0 {replica|temporal|all}" >&2
  exit 2
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing command: $1" >&2
    exit 1
  }
}

curl_json() {
  curl -sS -f "$@"
}

port_open() {
  local host="$1" port="$2"
  if command -v nc >/dev/null 2>&1; then
    nc -z "$host" "$port" >/dev/null 2>&1
  else
    bash -c "echo >/dev/tcp/${host}/${port}" 2>/dev/null
  fi
}

smoke_replica() {
  need_cmd curl
  need_cmd jq

  echo "== Health =="
  curl_json "${BASE}/v1/health" -H "X-API-Key: ${KEY}" | jq .

  local path_v content ts
  ts="$(date +%s)"
  path_v="root.smoke.replica.${ts}.$RANDOM"
  content="lag-smoke-$RANDOM"

  echo "== Store POST /v1/memories path=${path_v} =="
  curl_json -X POST "${BASE}/v1/memories" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${KEY}" \
    -d "$(jq -n --arg p "$path_v" --arg c "$content" '{path:$p, content:$c, metadata:{source:"local_smoke"}}')" | jq .

  echo "== GET immediately after (uses read pool: replica if DATABASE_READ_URL is set on API) =="
  local i found=""
  for i in $(seq 1 20); do
    if out="$(curl -sS "${BASE}/v1/memories/${path_v}" -H "X-API-Key: ${KEY}" 2>/dev/null)"; then
      if echo "$out" | jq -e --arg c "$content" '.content == $c' >/dev/null 2>&1; then
        echo "OK: GET sees the memory (attempt ${i}/20)"
        echo "$out" | jq .
        found=1
        break
      fi
    fi
    sleep 0.15
  done
  if [[ -z "$found" ]]; then
    echo "FAIL: after 20 attempts GET still does not return expected content (possible replication lag or API error)." >&2
    exit 1
  fi
}

temporal_cli_ok() {
  command -v temporal >/dev/null 2>&1
}

ensure_temporal_server() {
  if port_open "$TEMPORAL_HOST" "$TEMPORAL_PORT"; then
    return 0
  fi
  if [[ "${START_TEMPORAL_DEV:-}" != "1" ]]; then
    echo "No server on ${TEMPORAL_HOST}:${TEMPORAL_PORT}." >&2
    echo "Start Temporal (e.g. temporal server start-dev) or retry with START_TEMPORAL_DEV=1" >&2
    return 1
  fi
  if ! temporal_cli_ok; then
    echo "START_TEMPORAL_DEV=1 requires the Temporal CLI in PATH." >&2
    echo "Installazione: https://docs.temporal.io/cli (es. brew install temporal)" >&2
    return 1
  fi
  echo "== Starting temporal server start-dev in background (log: /tmp/pcmi-temporal-dev.log) =="
  nohup temporal server start-dev > /tmp/pcmi-temporal-dev.log 2>&1 &
  local pid=$!
  disown "$pid" 2>/dev/null || true
  local j
  for j in $(seq 1 45); do
    if port_open "$TEMPORAL_HOST" "$TEMPORAL_PORT"; then
      echo "Temporal listening on ${TEMPORAL_HOST}:${TEMPORAL_PORT}"
      return 0
    fi
    sleep 1
  done
  echo "Timeout waiting for Temporal. Last logs:" >&2
  tail -20 /tmp/pcmi-temporal-dev.log >&2 || true
  return 1
}

smoke_temporal() {
  need_cmd curl
  need_cmd jq
  need_cmd python3

  echo "== Health PCMI =="
  curl_json "${BASE}/v1/health" -H "X-API-Key: ${KEY}" | jq .

  ensure_temporal_server || return 1

  local td="${ROOT}/examples/temporal"
  local venv="${td}/.venv_smoke"

  if [[ "${SKIP_TEMPORAL_VENV:-}" == "1" ]]; then
    echo "== SKIP_TEMPORAL_VENV=1: using system python3 (import temporalio, httpx) =="
    python3 -c "import temporalio, httpx" 2>/dev/null || {
      echo "Installa: pip install temporalio httpx" >&2
      return 1
    }
  else
    echo "== Python venv + dependencies (smoke only) in ${venv} =="
    if [[ ! -d "$venv" ]]; then
      python3 -m venv "$venv"
    fi
    # shellcheck disable=SC1090
    source "${venv}/bin/activate"
    if python3 -c "import temporalio, httpx" 2>/dev/null; then
      echo "== Dependencies already installed in venv (skipping pip) =="
    else
      echo "== pip install (long timeout + retry; if it fails retry or use SKIP_TEMPORAL_VENV=1) =="
      local attempt ok=0
      for attempt in 1 2 3 4 5; do
        if PIP_DEFAULT_TIMEOUT=180 pip install -q --retries 10 -r "${td}/requirements.txt"; then
          ok=1
          break
        fi
        if [[ "$attempt" -eq 5 ]]; then
          echo "pip install failed after 5 attempts. Suggestions: stable VPN, PyPI mirror, or:" >&2
          echo "  cd examples/temporal && python3 -m venv .venv_smoke && . .venv_smoke/bin/activate && pip install -r requirements.txt" >&2
          echo "  or SKIP_TEMPORAL_VENV=1 if temporalio and httpx are already in the system python." >&2
          deactivate 2>/dev/null || true
          return 1
        fi
        echo "pip tentativo ${attempt}/5 fallito (rete/PyPI); attesa 5s..." >&2
        sleep 5
      done
    fi
  fi

  export PCMI_BASE_URL="$BASE"
  export PCMI_API_KEY="$KEY"
  export TEMPORAL_ADDRESS="${TEMPORAL_HOST}:${TEMPORAL_PORT}"

  echo "== Avvio worker Temporal (coda pcmi-demo) in background =="
  cd "$td"
  python3 worker.py &
  local wp=$!
  sleep 4

  local wf_path wf_body
  wf_path="root.temporal.smoke.$(date +%s).$RANDOM"
  wf_body="temporal-smoke-$RANDOM"
  echo "== Esecuzione workflow (starter) path=${wf_path} =="
  python3 starter.py "$wf_path" "$wf_body"

  kill "$wp" 2>/dev/null || true
  wait "$wp" 2>/dev/null || true
  if [[ "${SKIP_TEMPORAL_VENV:-}" != "1" ]]; then
    deactivate 2>/dev/null || true
  fi
  cd "$ROOT"
  echo "== Temporal smoke OK =="
}

main() {
  local mode="${1:-}"
  [[ -n "$mode" ]] || usage

  case "$mode" in
  replica)
    smoke_replica
    ;;
  temporal)
    smoke_temporal
    ;;
  all)
    smoke_replica
    if [[ "${SKIP_TEMPORAL:-}" == "1" ]]; then
      echo "SKIP_TEMPORAL=1: salto Temporal."
    else
      smoke_temporal
    fi
    ;;
  *)
    usage
    ;;
  esac
}

main "$@"
