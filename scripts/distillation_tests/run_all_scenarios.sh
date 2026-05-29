#!/usr/bin/env bash
# =============================================================================
# Runs all distillation scenarios in sequence.
# Compatible with bash 3.2 (macOS default) — uses parallel indexed arrays
# instead of `declare -A` (assoc array, bash 4+).
# =============================================================================
set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

GREEN="\033[1;32m"; YELLOW="\033[1;33m"; RED="\033[1;31m"; NC="\033[0m"

# Optimized order:
# - fast smoke first (fail fast if infra is broken)
# - 01 and 02 (reasonable LLM loads)
# - 05 (dedup, reuses state from 03)
# - 04 LAST because it intentionally saturates the OpenAI rate limit (200k TPM):
#   if run earlier, subsequent scenarios fail due to a full token bucket.
SCENARIOS=(
  "03_quick_smoke_100_to_10.sh"
  "01_full_coverage_1000_to_100.sh"
  "02_high_compression_1000_to_10.sh"
  "05_dedup_idempotency.sh"
  "04_cascade_rate_limit_stress.sh"
)

# Cooldown between scenarios (seconds). OpenAI token bucket is 60s rolling →
# 65s is enough to recover even after a "stressful" scenario.
# Override with env COOLDOWN_S=0 to disable.
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
  # Cooldown between scenarios (no cooldown after the last one)
  if (( i < ${#SCENARIOS[@]} - 1 )) && (( COOLDOWN_S > 0 )); then
    echo "[cooldown] sleeping ${COOLDOWN_S}s to let the OpenAI token bucket reset..."
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
