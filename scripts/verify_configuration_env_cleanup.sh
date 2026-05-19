#!/usr/bin/env bash
# Deprecated wrapper — use scripts/test_all_local.sh
exec "$(dirname "$0")/test_all_local.sh" "$@"
