#!/usr/bin/env bash
# =============================================================================
# Scenario 04 — Cascade auto-trigger stress test (intended to FAIL with 429s)
# =============================================================================
# Reproduces the "naive" behavior: does NOT stop the worker during ingest.
# Every memory.stored triggers 1 LLM call → 500 stores in cascade =
# saturation of the OpenAI rate limit (500 RPM / 200k TPM).
#
# - 500 SOC incidents (enough to saturate the limit)
# - --keep-worker → worker stays up, cascade active
# - DISTILLATION_BATCH_SIZE=10
# - Expected: distilled_count grows erratically, Worker 429 hits >> 0.
#
# Use case: empirically demonstrate why worker bypass is needed
#           during bulk ingest in production.
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "[scenario-04] this scenario is designed to generate 429s."
echo "[scenario-04] Expected: TEST FAIL on 'worker 429 hits' > 0."
echo

"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --num 500 \
  --keep-worker \
  --distill-batch-size 10 \
  --throttle-ms 100 \
  "$@" || {
    echo
    echo "[scenario-04] EXPECTED: fail with 429 hits > 0 (see metrics above)."
    exit 0
  }
