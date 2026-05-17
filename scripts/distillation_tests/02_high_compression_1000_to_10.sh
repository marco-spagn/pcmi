#!/usr/bin/env bash
# =============================================================================
# Scenario 02 — High compression 1000 raw → 10 distilled (compression 100:1)
# =============================================================================
# - 1000 incidenti SOC, seed 42
# - DISTILLATION_BATCH_SIZE=100 (shard di 100 record cadauno)
# - 10 shard totali → 10 refine events → 10 distilled "ricchi"
# - Ogni distilled summary aggrega 100 incidenti reali sotto un cluster
# - Atteso: distilled_count Δ=10, raw_sources=1000, coverage=100%
#
# Use case: vedere come cambia la qualità del summary con più input al LLM.
# (Più input ≈ summary più articolato + costo per call più alto in token.)
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --num 1000 \
  --distill-batch-size 100 \
  "$@"
