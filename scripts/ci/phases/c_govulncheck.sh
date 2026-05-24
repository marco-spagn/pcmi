#!/usr/bin/env bash
# Phase C — govulncheck (CI job: security). Skipped when SKIP_GOVCHECK=1.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

if [[ "${SKIP_GOVCHECK:-}" == "1" ]]; then
  echo "[ci] SKIP govulncheck (SKIP_GOVCHECK=1)"
  exit 0
fi

go run golang.org/x/vuln/cmd/govulncheck@latest ./...
