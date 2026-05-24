#!/usr/bin/env bash
# Resolve API version from internal/version/version.go (single source of truth).
# Source from other scripts:  source "$(dirname "$0")/resolve_version.sh"
# Or run to print:           scripts/ci/resolve_version.sh
set -euo pipefail

_ci_resolve_root() {
  local here="${BASH_SOURCE[0]}"
  while [[ -L "$here" ]]; do
    here="$(readlink "$here")"
  done
  cd "$(dirname "$here")/../.." && pwd
}

if [[ -z "${PCMI_REPO_ROOT:-}" ]]; then
  PCMI_REPO_ROOT="$(_ci_resolve_root)"
fi

_version_go="${PCMI_REPO_ROOT}/internal/version/version.go"
if [[ ! -f "$_version_go" ]]; then
  echo "resolve_version: missing $_version_go" >&2
  exit 1
fi

_tag="$(
  grep -E '^\s*Tag\s*=' "$_version_go" | head -1 | sed -E 's/.*"([^"]+)".*/\1/'
)"
if [[ -z "$_tag" ]]; then
  echo "resolve_version: could not parse Tag from $_version_go" >&2
  exit 1
fi

export PCMI_EXPECT_VERSION="${PCMI_EXPECT_VERSION:-$_tag}"
export EXPECT_API_VERSION="${EXPECT_API_VERSION:-$_tag}"

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  echo "$PCMI_EXPECT_VERSION"
fi
