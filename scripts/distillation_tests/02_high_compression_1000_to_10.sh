#!/usr/bin/env bash
# =============================================================================
# Scenario 02 — High compression 1000 raw → 10 distilled (compression 100:1)
# =============================================================================
# - 1000 SOC incidents, seed 42
# - DISTILLATION_BATCH_SIZE=100 (shards of 100 records each)
# - 10 total shards → 10 refine events → 10 "rich" distilled entries
# - Each distilled summary aggregates 100 real incidents under a cluster
# - Expected: distilled_count Δ=10, raw_sources=1000, coverage=100%
#
# Use case: see how summary quality changes with more LLM input.
# (More input ≈ more detailed summary + higher token cost per call.)
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --num 1000 \
  --distill-batch-size 100 \
  "$@"
