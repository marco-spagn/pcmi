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
    echo "Manca il comando: $1" >&2
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

  echo "== GET subito dopo (usa il pool di lettura: replica se DATABASE_READ_URL è impostato sull'API) =="
  local i found=""
  for i in $(seq 1 20); do
    if out="$(curl -sS "${BASE}/v1/memories/${path_v}" -H "X-API-Key: ${KEY}" 2>/dev/null)"; then
      if echo "$out" | jq -e --arg c "$content" '.content == $c' >/dev/null 2>&1; then
        echo "OK: GET vede la memoria (tentativo ${i}/20)"
        echo "$out" | jq .
        found=1
        break
      fi
    fi
    sleep 0.15
  done
  if [[ -z "$found" ]]; then
    echo "FAIL: dopo 20 tentativi GET non restituisce ancora content atteso (possibile replication lag o errore API)." >&2
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
    echo "Nessun server su ${TEMPORAL_HOST}:${TEMPORAL_PORT}." >&2
    echo "Avvia Temporal (es. temporal server start-dev) oppure ripeti con START_TEMPORAL_DEV=1" >&2
    return 1
  fi
  if ! temporal_cli_ok; then
    echo "START_TEMPORAL_DEV=1 richiede la CLI Temporal nel PATH." >&2
    echo "Installazione: https://docs.temporal.io/cli (es. brew install temporal)" >&2
    return 1
  fi
  echo "== Avvio temporal server start-dev in background (log: /tmp/pcmi-temporal-dev.log) =="
  nohup temporal server start-dev > /tmp/pcmi-temporal-dev.log 2>&1 &
  local pid=$!
  disown "$pid" 2>/dev/null || true
  local j
  for j in $(seq 1 45); do
    if port_open "$TEMPORAL_HOST" "$TEMPORAL_PORT"; then
      echo "Temporal in ascolto su ${TEMPORAL_HOST}:${TEMPORAL_PORT}"
      return 0
    fi
    sleep 1
  done
  echo "Timeout in attesa di Temporal. Ultimi log:" >&2
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
  echo "== Python venv + dipendenze (solo smoke) in ${venv} =="
  python3 -m venv "$venv"
  # shellcheck disable=SC1090
  source "${venv}/bin/activate"
  pip install -q -r "${td}/requirements.txt"

  export PCMI_BASE_URL="$BASE"
  export PCMI_API_KEY="$KEY"
  export TEMPORAL_ADDRESS="${TEMPORAL_HOST}:${TEMPORAL_PORT}"

  echo "== Avvio worker Temporal (coda pcmi-demo) in background =="
  cd "$td"
  python worker.py &
  local wp=$!
  sleep 4

  local wf_path wf_body
  wf_path="root.temporal.smoke.$(date +%s).$RANDOM"
  wf_body="temporal-smoke-$RANDOM"
  echo "== Esecuzione workflow (starter) path=${wf_path} =="
  python starter.py "$wf_path" "$wf_body"

  kill "$wp" 2>/dev/null || true
  wait "$wp" 2>/dev/null || true
  deactivate 2>/dev/null || true
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
