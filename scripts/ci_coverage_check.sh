#!/usr/bin/env bash
#
# scripts/ci_coverage_check.sh
#
# Enforces coverage thresholds on a Go coverage profile (coverage.out) and
# emits enterprise-friendly artefacts:
#
#   - plain text table          (always, on stdout + optional file)
#   - markdown table            (when COVERAGE_MD_OUT is set; used by CI for
#                                $GITHUB_STEP_SUMMARY and sticky PR comment)
#   - shields.io badge URL      (when COVERAGE_BADGE_OUT is set; static URL
#                                that the README can point to without an
#                                external coverage service)
#
# The script parses the standard `go test -coverprofile=...` output without
# requiring `go tool cover`, so it runs identically inside CI, act, and a
# developer's machine.
#
# Thresholds (override via env):
#   COVERAGE_MIN_TOTAL              global floor (default: 22)
#   COVERAGE_PKG_FLOORS             comma-separated pkg:pct overrides,
#                                   e.g. "config:70,event:70,eventschema:85,metrics:70"
#   COVERAGE_FAIL_ON_MISSING_FILE   "true" → fail if coverage.out is missing (default: true)
#   COVERAGE_OUT                    path to coverage profile (default: coverage.out)
#   COVERAGE_SUMMARY_OUT            optional path to write the plain-text summary
#   COVERAGE_MD_OUT                 optional path to write a markdown summary
#   COVERAGE_BADGE_OUT              optional path to write the shields.io URL
#   COVERAGE_OMIT_PROTOBUF          "true" → exclude internal/grpc/pcmiv1/*.pb.go
#                                   from stmt totals (generated protobuf only)
#
# Exit codes:
#   0   all thresholds met
#   1   one or more thresholds not met
#   2   invalid invocation / coverage file missing

set -euo pipefail

COVERAGE_OUT="${COVERAGE_OUT:-coverage.out}"
COVERAGE_MIN_TOTAL="${COVERAGE_MIN_TOTAL:-22}"
COVERAGE_PKG_FLOORS="${COVERAGE_PKG_FLOORS:-config:70,event:70,eventschema:85,metrics:70,version:80}"
COVERAGE_FAIL_ON_MISSING_FILE="${COVERAGE_FAIL_ON_MISSING_FILE:-true}"
COVERAGE_SUMMARY_OUT="${COVERAGE_SUMMARY_OUT:-}"
COVERAGE_MD_OUT="${COVERAGE_MD_OUT:-}"
COVERAGE_BADGE_OUT="${COVERAGE_BADGE_OUT:-}"
COVERAGE_OMIT_PROTOBUF="${COVERAGE_OMIT_PROTOBUF:-false}"

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
SUMMARY="$(awk -v omit="$COVERAGE_OMIT_PROTOBUF" '
NR == 1 { next } # skip the "mode: <m>" header
{
  if (NF != 3) next
  stmts = $2 + 0
  count = $3 + 0
  fp = $1
  sub(/:[0-9]+\.[0-9]+,[0-9]+\.[0-9]+$/, "", fp)
  if (omit == "true" && fp ~ /\/internal\/grpc\/pcmiv1\// && fp ~ /\.pb\.go$/) next

  pkg = "(other)"
  if (match(fp, /\/internal\/[^\/]+\//)) {
    pkg = substr(fp, RSTART + 10)
    sub(/\/.*$/, "", pkg)
  } else if (match(fp, /\/cmd\/[^\/]+\//)) {
    pkg = "cmd-" substr(fp, RSTART + 5)
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

# ─── Plain-text report (stdout + optional file) ───────────────────────────────

report_text() {
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

REPORT="$(report_text)"
printf '%s\n' "$REPORT"

if [ -n "$COVERAGE_SUMMARY_OUT" ]; then
  printf '%s\n' "$REPORT" > "$COVERAGE_SUMMARY_OUT"
fi

# ─── Markdown report (for GITHUB_STEP_SUMMARY + PR comment) ───────────────────

# Coverage thresholds for emoji decoration in markdown.
# Note: ✅ for >=70%, 🟡 for 30-70%, 🔴 for <30%.
md_emoji() {
  awk -v p="$1" 'BEGIN { if (p+0 >= 70) print "✅"; else if (p+0 >= 30) print "🟡"; else print "🔴" }'
}

GLOBAL_PCT="$(printf '%s\n' "$SUMMARY" | awk -F'\t' '$1=="GLOBAL" { if ($3>0) printf "%.2f", ($4/$3)*100; else print "0.00" }')"
GLOBAL_EMOJI="$(md_emoji "$GLOBAL_PCT")"

if [ -n "$COVERAGE_MD_OUT" ]; then
  {
    echo "## ${GLOBAL_EMOJI} Test coverage: \`${GLOBAL_PCT}%\`"
    echo ""
    echo "_Global statement coverage; threshold = **${COVERAGE_MIN_TOTAL}%** (\`scripts/ci_coverage_check.sh\`)._"
    echo ""
    echo "| Package | Stmts | Covered | Coverage |"
    echo "|---|---:|---:|---:|"
    printf '%s\n' "$SUMMARY" | awk -F'\t' '$1=="PKG" {
      pct = ($3 > 0) ? ($4/$3)*100 : 0
      emoji = (pct >= 70) ? "✅" : (pct >= 30 ? "🟡" : "🔴")
      printf "| `%s` | %d | %d | %s %.1f%% |\n", $2, $3, $4, emoji, pct
    }' | sort
    echo "| **Global** | $(printf '%s\n' "$SUMMARY" | awk -F'\t' '$1=="GLOBAL" {printf "**%d** | **%d** | **%s %.1f%%**", $3, $4, "'"$GLOBAL_EMOJI"'", ($4/$3)*100}') |"
    echo ""
    echo "<sub>Generated by \`scripts/ci_coverage_check.sh\` — bump floors in the same PR that lifts coverage.</sub>"
  } > "$COVERAGE_MD_OUT"
fi

# ─── Shields.io badge URL ─────────────────────────────────────────────────────

# Static shields.io URL — color is chosen from the rounded percentage so the
# README stays in sync with reality after every CI run that commits the badge.
if [ -n "$COVERAGE_BADGE_OUT" ]; then
  # shields.io colour palette (matches their defaults).
  BADGE_COLOR="$(awk -v p="$GLOBAL_PCT" 'BEGIN {
    p += 0
    if (p >= 90) print "brightgreen"
    else if (p >= 75) print "green"
    else if (p >= 60) print "yellowgreen"
    else if (p >= 40) print "yellow"
    else if (p >= 20) print "orange"
    else print "red"
  }')"
  # Round to 1 decimal for the badge label.
  BADGE_PCT="$(awk -v p="$GLOBAL_PCT" 'BEGIN { printf "%.1f", p+0 }')"
  BADGE_URL="https://img.shields.io/badge/coverage-${BADGE_PCT}%25-${BADGE_COLOR}.svg"
  printf '%s\n' "$BADGE_URL" > "$COVERAGE_BADGE_OUT"
fi

# ─── Threshold enforcement ────────────────────────────────────────────────────

# Compare floating-point percentages without bc/python.
ge() { awk -v a="$1" -v b="$2" 'BEGIN { exit !(a + 0 >= b + 0) }'; }

fail=0

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
