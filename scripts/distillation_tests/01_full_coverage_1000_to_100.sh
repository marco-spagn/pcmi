#!/usr/bin/env bash
# =============================================================================
# Scenario 01 — Full coverage 1000 raw → 100 distilled (compression 10:1)
# =============================================================================
# - 1000 incidenti SOC, seed 42 deterministico
# - sharding 10 record per shard → 100 shard
# - DISTILLATION_BATCH_SIZE=10 (corrisponde allo shard size)
# - 100 refine events Redis, uno per shard
# - Atteso: distilled_count Δ=100, raw_sources=1000, coverage=100%, 429=0
#
# Durata stimata: ~3-4 min (ingest 30s + 100 refine × 1.2s + LLM tail).
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --num 1000 \
  --distill-batch-size 10 \
  "$@"
