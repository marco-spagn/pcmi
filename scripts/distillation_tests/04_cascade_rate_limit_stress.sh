#!/usr/bin/env bash
# =============================================================================
# Scenario 04 — Cascade auto-trigger stress test (intended to FAIL with 429s)
# =============================================================================
# Riproduce il comportamento "naive": NON ferma il worker durante l'ingest.
# Ogni memory.stored triggera 1 chiamata LLM → 500 store in cascata =
# saturazione del rate limit OpenAI (500 RPM / 200k TPM).
#
# - 500 incidenti SOC (basta per saturare il limit)
# - --keep-worker → worker resta su, cascata attiva
# - DISTILLATION_BATCH_SIZE=10
# - Atteso: distilled_count cresce in modo erratico, Worker 429 hits >> 0.
#
# Use case: dimostrare empiricamente perché serve il bypass del worker
#           durante il bulk ingest in produzione.
# =============================================================================
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "[scenario-04] questo scenario è progettato per generare 429s."
echo "[scenario-04] Atteso: TEST FAIL su 'worker 429 hits' > 0."
echo

"${SCRIPT_DIR}/../run_pcmi_distillation_test.sh" \
  --no-build \
  --num 500 \
  --keep-worker \
  --distill-batch-size 10 \
  --throttle-ms 100 \
  "$@" || {
    echo
    echo "[scenario-04] ATTESO: fail con 429 hits > 0 (vedi metriche sopra)."
    exit 0
  }
