#!/usr/bin/env bash
# =============================================================================
# Scenario 05 — Idempotenza distillation (dedup su source_entry_ids)
# =============================================================================
# Verifica che il check `hasDuplicateDistillation` del worker rifiuti
# tentativi ripetuti di distillare gli stessi 10 record.
#
# Flusso:
#   1) Esegue lo scenario 03 (100→10 distilled).
#   2) NON fa teardown.
#   3) Ripubblica gli stessi 10 refine events.
#   4) Verifica che distilled_count NON cresca (idempotenza).
#
# Use case: validare la garanzia "exactly-once distillation per shard".
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

GREEN="\033[1;32m"; YELLOW="\033[1;33m"; RED="\033[1;31m"; NC="\033[0m"
ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[ERR]${NC} $*" >&2; }

# Fase 1: smoke standard (NON fa teardown)
echo "[scenario-05] fase 1/3 — primo round di distillation (100 → 10)"
"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --no-teardown \
  --num 100 \
  --distill-batch-size 10

# Fase 2: rilegge stats correnti
TENANT_ID="${TENANT_ID:-a1b2c3d4-e5f6-7890-abcd-ef1234567890}"
PCMI_BASE_URL="${PCMI_BASE_URL:-http://localhost:8000}"
KEY_NAME="soc-bulk-loader"
ADMIN_API_KEY="${PCMI_ADMIN_API_KEY:-testkey123}"
KEY_ID="$(curl -fsS -H "X-API-Key: ${ADMIN_API_KEY}" \
  "${PCMI_BASE_URL}/v1/admin/api-keys?tenant_id=${TENANT_ID}&limit=200" \
  | jq -r --arg n "$KEY_NAME" '.api_keys[] | select(.name==$n) | .id' | head -1)"
KEY_RAW="$(curl -fsS -X POST -H "X-API-Key: ${ADMIN_API_KEY}" -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg n "$KEY_NAME" '{name:$n}')" \
  "${PCMI_BASE_URL}/v1/admin/api-keys/${KEY_ID}/rotate" | jq -r '.api_key')"

PRE="$(curl -fsS -H "X-API-Key: ${KEY_RAW}" "${PCMI_BASE_URL}/v1/stats" | jq -r '.distilled_count')"
echo "[scenario-05] fase 2/3 — distilled_count attuale = ${PRE}"

# Fase 3: ripubblica gli stessi 10 refine events
# Il check di dedup del worker (hasDuplicateDistillation) avviene DOPO la
# LLM call: quindi il worker fa comunque la chiamata, poi si accorge che
# ha gli stessi source_entry_ids e fa skip dell'INSERT.
# Risultato atteso: distilled_count invariato.
echo "[scenario-05] fase 3/3 — riapplica 10 refine events identici"
for ((i=0; i<10; i++)); do
  PREFIX="$(printf "root.security.incidents.soc.shard_%03d" "$i")"
  PAYLOAD="$(jq -nc --arg t "$TENANT_ID" --arg p "$PREFIX" \
    '{Type:"memory.refine.requested", Payload:{tenant_id:$t, path_prefix:$p, reason:"dedup_retry"}}')"
  ( cd "$ROOT" && docker compose exec -T redis redis-cli PUBLISH memory_events "$PAYLOAD" >/dev/null )
  sleep 1.2
done
echo "[scenario-05] aspetto 30s che il worker finisca le 10 LLM calls + dedup check…"
sleep 30

POST="$(curl -fsS -H "X-API-Key: ${KEY_RAW}" "${PCMI_BASE_URL}/v1/stats" | jq -r '.distilled_count')"
echo "[scenario-05] dopo retry: distilled_count = ${POST}"

if [[ "$PRE" == "$POST" ]]; then
  ok "Idempotenza confermata: distilled_count invariato (${PRE} = ${POST})."
  EXIT=0
else
  err "Dedup violata: distilled_count è cambiato (${PRE} → ${POST})."
  EXIT=1
fi

echo "[scenario-05] teardown finale"
( cd "$ROOT" && docker compose down -v --remove-orphans >/dev/null 2>&1 ) || true
exit $EXIT
