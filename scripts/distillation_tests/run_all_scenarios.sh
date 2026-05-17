#!/usr/bin/env bash
# =============================================================================
# Esegue in sequenza tutti gli scenari di distillation.
# Compatibile con bash 3.2 (default macOS) — usa array indicizzati paralleli
# invece di `declare -A` (assoc array, bash 4+).
# =============================================================================
set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

GREEN="\033[1;32m"; YELLOW="\033[1;33m"; RED="\033[1;31m"; NC="\033[0m"

# Ordine ottimizzato:
# - smoke veloce per primo (fail fast se infra è rotta)
# - 01 e 02 (carichi LLM ragionevoli)
# - 05 (dedup, riusa lo stato di 03)
# - 04 ULTIMO perché satura volutamente il rate limit OpenAI (200k TPM):
#   se messo prima, i successivi falliscono per token bucket pieno.
SCENARIOS=(
  "03_quick_smoke_100_to_10.sh"
  "01_full_coverage_1000_to_100.sh"
  "02_high_compression_1000_to_10.sh"
  "05_dedup_idempotency.sh"
  "04_cascade_rate_limit_stress.sh"
)

# Cooldown tra scenari (secondi). Il token bucket di OpenAI è 60s rolling →
# 65s sono sufficienti a recuperare anche dopo uno scenario "stressante".
# Override con env COOLDOWN_S=0 per disattivare.
COOLDOWN_S="${COOLDOWN_S:-65}"

# Array paralleli (indice = posizione in SCENARIOS)
RESULTS=()
DURATIONS=()

run_scenario() {
  local name="$1"
  local start_ts end_ts elapsed
  start_ts="$(date +%s)"
  if "${SCRIPT_DIR}/${name}"; then
    RESULTS+=("PASS")
  else
    RESULTS+=("FAIL")
  fi
  end_ts="$(date +%s)"
  elapsed=$(( end_ts - start_ts ))
  DURATIONS+=("${elapsed}s")
}

for i in "${!SCENARIOS[@]}"; do
  s="${SCENARIOS[$i]}"
  echo
  echo -e "${YELLOW}════════════════════════════════════════════════════════════════════${NC}"
  echo -e "${YELLOW}  Running ${s}${NC}"
  echo -e "${YELLOW}════════════════════════════════════════════════════════════════════${NC}"
  run_scenario "$s"
  # Cooldown tra scenari (no cooldown dopo l'ultimo)
  if (( i < ${#SCENARIOS[@]} - 1 )) && (( COOLDOWN_S > 0 )); then
    echo "[cooldown] dormo ${COOLDOWN_S}s per far resettare il token bucket OpenAI…"
    sleep "$COOLDOWN_S"
  fi
done

echo
echo -e "${GREEN}════════════════════════════════════════════════════════════════════${NC}"
echo "  ALL SCENARIOS — SUMMARY"
echo -e "${GREEN}════════════════════════════════════════════════════════════════════${NC}"

exit_code=0
for i in "${!SCENARIOS[@]}"; do
  status="${RESULTS[$i]}"
  duration="${DURATIONS[$i]}"
  if [[ "$status" == "PASS" ]]; then
    printf "  ${GREEN}%-5s${NC}  %-8s  %s\n" "$status" "$duration" "${SCENARIOS[$i]}"
  else
    printf "  ${RED}%-5s${NC}  %-8s  %s\n" "$status" "$duration" "${SCENARIOS[$i]}"
    exit_code=1
  fi
done

exit "$exit_code"
