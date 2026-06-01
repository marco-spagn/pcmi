#!/usr/bin/env bash
# Shared coverage gate env (GitHub job `go` + local ci_like_github Phase F).
set -euo pipefail

export COVERAGE_MIN_TOTAL="${COVERAGE_MIN_TOTAL:-41}"
export COVERAGE_PKG_FLOORS="${COVERAGE_PKG_FLOORS:-config:70,event:70,eventschema:85,repository:50,service:70,worker:45,metrics:80}"
export COVERAGE_OMIT_PROTOBUF="${COVERAGE_OMIT_PROTOBUF:-true}"
export COVERAGE_MD_OUT="${COVERAGE_MD_OUT:-coverage-summary.md}"
export COVERAGE_BADGE_OUT="${COVERAGE_BADGE_OUT:-coverage-badge.txt}"
export COVERAGE_ENDPOINT_OUT="${COVERAGE_ENDPOINT_OUT:-badges/coverage.json}"
