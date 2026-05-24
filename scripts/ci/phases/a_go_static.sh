#!/usr/bin/env bash
# Phase A — go build, vet, config audit (CI job: go, first steps).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

go build ./...
go vet ./...
go test -count=1 ./internal/config/... \
  -run 'TestNoOSGetenvOutsideConfig|TestEnvExampleStaysInSyncWithConfig'
