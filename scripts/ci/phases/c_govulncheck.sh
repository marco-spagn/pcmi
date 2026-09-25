#!/usr/bin/env bash
# Phase C — govulncheck (CI job: security). Skipped when SKIP_GOVCHECK=1.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

if [[ "${SKIP_GOVCHECK:-}" == "1" ]]; then
  echo "[ci] SKIP govulncheck (SKIP_GOVCHECK=1)"
  exit 0
fi

# Pinned: govulncheck@latest (golang.org/x/vuln >= v1.8.0) requires Go 1.26,
# which fails under GOTOOLCHAIN=local with the go.mod toolchain (go1.25.x).
# Bump together with the go.mod toolchain directive.
GOVULNCHECK_VERSION="${GOVULNCHECK_VERSION:-v1.7.0}"
go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" ./...
