#!/usr/bin/env bash
# Deprecated: moved to scripts/e2e/test_pcmi.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo "note: test_pcmi.sh moved to scripts/e2e/test_pcmi.sh" >&2
exec "${ROOT}/scripts/e2e/test_pcmi.sh" "$@"
