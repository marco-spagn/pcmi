#!/usr/bin/env bash
# =============================================================================
# Scenario 03 — Quick smoke 100 raw → 10 distilled
# =============================================================================
# Smoke veloce per CI / iterazione locale. Stessa pipeline ma molto piccola.
# - 100 incidenti
# - DISTILLATION_BATCH_SIZE=10 → 10 shard → 10 distilled
# - Durata ~1 minuto
#
# Use case: pre-merge smoke, validare che la pipeline regga.
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --num 100 \
  --distill-batch-size 10 \
  "$@"
