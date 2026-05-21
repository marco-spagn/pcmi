#!/usr/bin/env bash
# Deprecated wrapper — use scripts/test_all_local.sh (Docker phases B6–B13)
exec "$(dirname "$0")/test_all_local.sh" "$@"
