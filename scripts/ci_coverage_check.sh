#!/usr/bin/env bash
#
# scripts/ci_coverage_check.sh
#
# Enforces coverage thresholds on a Go coverage profile (coverage.out).
#
# The script parses the standard `go test -coverprofile=...` output without
# requiring `go tool cover`, so it runs identically inside CI, act, and a
# developer's machine. It computes:
#   - global statement coverage
#   - per-package statement coverage (grouped by internal/<pkg>/...)
#
# Thresholds (override via env):
#   COVERAGE_MIN_TOTAL              global floor (default: 22)
#   COVERAGE_PKG_FLOORS             comma-separated pkg:pct overrides,
#                                   e.g. "config:70,event:70,eventschema:85,metrics:70"
#   COVERAGE_FAIL_ON_MISSING_FILE   "true" → fail if coverage.out is missing (default: true)
#   COVERAGE_OUT                    path to coverage profile (default: coverage.out)
#   COVERAGE_SUMMARY_OUT            optional path to write a plain-text summary
#
# Exit codes:
#   0   all thresholds met
#   1   one or more thresholds not met
#   2   invalid invocation / coverage file missing
#
# This script is intentionally pure POSIX-ish bash + awk so it works in the
# minimal CI runner image with no extra toolchain.

set -euo pipefail

COVERAGE_OUT="${COVERAGE_OUT:-coverage.out}"
COVERAGE_MIN_TOTAL="${COVERAGE_MIN_TOTAL:-22}"
COVERAGE_PKG_FLOORS="${COVERAGE_PKG_FLOORS:-config:70,event:70,eventschema:85,metrics:70,version:80}"
COVERAGE_FAIL_ON_MISSING_FILE="${COVERAGE_FAIL_ON_MISSING_FILE:-true}"
COVERAGE_SUMMARY_OUT="${COVERAGE_SUMMARY_OUT:-}"

if [ ! -f "$COVERAGE_OUT" ]; then
  if [ "$COVERAGE_FAIL_ON_MISSING_FILE" = "true" ]; then
    echo "::error::coverage profile $COVERAGE_OUT not found" >&2
    exit 2
  fi
  echo "::warning::coverage profile $COVERAGE_OUT not found — skipping" >&2
  exit 0
fi

# Compute coverage with awk reading the profile directly:
#   - each line is:  <file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmt> <count>
#   - pkg = first directory after "internal/" (or cmd-<bin> for cmd/*)
SUMMARY="$(awk '
NR == 1 { next } # skip the "mode: <m>" header
{
  if (NF != 3) next
  stmts = $2 + 0
  count = $3 + 0
  file  = $1

  pkg = "(other)"
  if (match(file, /\/internal\/[^\/]+\//)) {
    pkg = substr(file, RSTART + 10)
    sub(/\/.*$/, "", pkg)
  } else if (match(file, /\/cmd\/[^\/]+\//)) {
    pkg = "cmd-" substr(file, RSTART + 5)
    sub(/\/.*$/, "", pkg)
  }

  totalStmts[pkg] += stmts
  grandStmts     += stmts
  if (count > 0) {
    covStmts[pkg] += stmts
    grandCov      += stmts
  }
  seen[pkg] = 1
}
END {
  for (pkg in seen) {
    printf "PKG\t%s\t%d\t%d\n", pkg, totalStmts[pkg]+0, covStmts[pkg]+0
  }
  printf "GLOBAL\t-\t%d\t%d\n", grandStmts+0, grandCov+0
}
' "$COVERAGE_OUT")"

if [ -z "$SUMMARY" ]; then
  echo "::error::coverage profile $COVERAGE_OUT contained no entries" >&2
  exit 2
fi

# Pretty-print the per-package table + grand total to stdout (and optional file).
report() {
  printf "%-20s %10s %10s %10s\n" "PACKAGE" "STMTS" "COVERED" "COVERAGE"
  printf "%-20s %10s %10s %10s\n" "-------" "-----" "-------" "--------"
  printf '%s\n' "$SUMMARY" | awk -F'\t' '$1=="PKG" {
    pct = ($3 > 0) ? ($4/$3)*100 : 0
    printf "%-20s %10d %10d %9.1f%%\n", $2, $3, $4, pct
  }' | sort
  echo "----"
  printf '%s\n' "$SUMMARY" | awk -F'\t' '$1=="GLOBAL" {
    pct = ($3 > 0) ? ($4/$3)*100 : 0
    printf "%-20s %10d %10d %9.1f%%\n", "GLOBAL", $3, $4, pct
  }'
}

REPORT="$(report)"
printf '%s\n' "$REPORT"

if [ -n "$COVERAGE_SUMMARY_OUT" ]; then
  printf '%s\n' "$REPORT" > "$COVERAGE_SUMMARY_OUT"
fi

# ─── Threshold enforcement ────────────────────────────────────────────────────

# Compare floating-point percentages without bc/python.
ge() { awk -v a="$1" -v b="$2" 'BEGIN { exit !(a + 0 >= b + 0) }'; }

fail=0

# Global floor.
GLOBAL_PCT="$(printf '%s\n' "$SUMMARY" | awk -F'\t' '$1=="GLOBAL" { if ($3>0) printf "%.2f", ($4/$3)*100; else print "0.00" }')"

if ! ge "$GLOBAL_PCT" "$COVERAGE_MIN_TOTAL"; then
  echo "::error::global coverage ${GLOBAL_PCT}% < required ${COVERAGE_MIN_TOTAL}%" >&2
  fail=1
else
  echo "ok: global coverage ${GLOBAL_PCT}% >= ${COVERAGE_MIN_TOTAL}%"
fi

# Per-package floors.
if [ -n "$COVERAGE_PKG_FLOORS" ]; then
  IFS=',' read -r -a floors <<< "$COVERAGE_PKG_FLOORS"
  for entry in "${floors[@]}"; do
    [ -z "$entry" ] && continue
    pkg="${entry%%:*}"
    pct="${entry##*:}"
    if [ "$pkg" = "$entry" ] || [ -z "$pkg" ] || [ -z "$pct" ]; then
      echo "::error::malformed COVERAGE_PKG_FLOORS entry: $entry" >&2
      fail=1
      continue
    fi

    PKG_PCT="$(printf '%s\n' "$SUMMARY" | awk -F'\t' -v p="$pkg" '$1=="PKG" && $2==p { if ($3>0) printf "%.2f", ($4/$3)*100; else print "0.00" }')"
    if [ -z "$PKG_PCT" ]; then
      echo "::warning::package floor for '$pkg' configured but package not present in coverage profile"
      continue
    fi

    if ! ge "$PKG_PCT" "$pct"; then
      echo "::error::package '$pkg' coverage ${PKG_PCT}% < required ${pct}%" >&2
      fail=1
    else
      echo "ok: package '$pkg' coverage ${PKG_PCT}% >= ${pct}%"
    fi
  done
fi

exit "$fail"
