#!/usr/bin/env bash
# Simple entrypoint for distillation E2E (forwards to run_pcmi_distillation_test.sh).
#
# Examples:
#   ./scripts/distill_e2e.sh --preset soc --num 1000 --seed 42
#   ./scripts/distill_e2e.sh --preset finance --num 200 --seed 1 --no-build
#   ./scripts/distill_e2e.sh --preset custom --domain "insurance claims" --num 80 --llm
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec "${ROOT}/scripts/run_pcmi_distillation_test.sh" "$@"
