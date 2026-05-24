#!/usr/bin/env bash
# Phase F — go test -race -tags=integration ./internal/... + coverage gate.
# Expects DATABASE_URL and migrated Postgres. Used by ci_like_github.sh.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

export DATABASE_URL="${DATABASE_URL:-postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable}"
export PCMI_ENCRYPTION_KEY="${PCMI_ENCRYPTION_KEY:-01234567890123456789012345678901}"
# GitHub CI runs SSE httptest; local ci_like_github sets PCMI_SKIP_SSE_HTTPTEST=1 before Phase F.

GT_TIMEOUT="${PCMI_GO_TEST_TIMEOUT:-45m}"
GT_ARGS=( -count=1 -tags=integration )
if [[ "${CI_LIKE_NO_RACE:-}" != "1" ]]; then
  GT_ARGS=( -race "${GT_ARGS[@]}" )
fi
if [[ -n "${PCMI_GO_TEST_P:-}" ]]; then
  GT_ARGS+=( -p "${PCMI_GO_TEST_P}" )
fi
GT_ARGS+=( -timeout "$GT_TIMEOUT" )
if [[ "${CI_LIKE_GO_VERBOSE:-}" == "1" ]]; then
  GT_ARGS+=( -v )
fi
GT_ARGS+=( -coverprofile=coverage.out -covermode=atomic )

_heartbeat_pid=
if [[ "${CI_LIKE_HEARTBEAT_SECS:-0}" =~ ^[1-9][0-9]*$ ]]; then
  (
    while sleep "${CI_LIKE_HEARTBEAT_SECS}"; do
      ts="$(date +%H:%M:%S)"
      lines="$(pgrep -fl '[g]o test' 2>/dev/null | head -5 || true)"
      if [[ -n "$lines" ]]; then
        echo "[ci] heartbeat ${ts} — go test:" >&2
        echo "$lines" >&2
      else
        echo "[ci] heartbeat ${ts} — (compile/link or gap between packages)" >&2
      fi
    done
  ) &
  _heartbeat_pid=$!
fi

_go_ec=0
go test "${GT_ARGS[@]}" ./internal/... || _go_ec=$?

if [[ -n "${_heartbeat_pid:-}" ]]; then
  kill "${_heartbeat_pid}" 2>/dev/null || true
  wait "${_heartbeat_pid}" 2>/dev/null || true
fi
[[ $_go_ec -eq 0 ]] || exit "$_go_ec"

if [[ "${SKIP_COVERAGE:-}" != "1" ]]; then
  # shellcheck source=../coverage_env.sh
  source "$ROOT/scripts/ci/coverage_env.sh"
  chmod +x scripts/ci_coverage_check.sh
  bash scripts/ci_coverage_check.sh
fi
