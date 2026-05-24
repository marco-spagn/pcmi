#!/usr/bin/env bash
# Phase B — golangci-lint (CI job: golangci-lint). Skipped when SKIP_LINT=1.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

if [[ "${SKIP_LINT:-}" == "1" ]]; then
  echo "[ci] SKIP golangci-lint (SKIP_LINT=1)"
  exit 0
fi

GOLANGCI_IMAGE="${GOLANGCI_IMAGE:-golangci/golangci-lint:v2.12.2}"
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run ./...
elif docker info >/dev/null 2>&1; then
  docker run --rm -v "$ROOT:/app" -w /app "$GOLANGCI_IMAGE" golangci-lint run ./...
else
  echo "[ci] SKIP golangci-lint — install golangci-lint or start Docker" >&2
  exit 1
fi
