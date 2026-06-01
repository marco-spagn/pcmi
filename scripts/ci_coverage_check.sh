#!/usr/bin/env bash
#
# scripts/ci_coverage_check.sh
#
# Enforces coverage thresholds on a Go coverage profile (coverage.out) and
# emits enterprise-friendly artefacts:
#
#   - plain text table          (always, on stdout + optional file)
#   - markdown table            (when COVERAGE_MD_OUT is set; used by CI for
#                                $GITHUB_STEP_SUMMARY and sticky PR comment).
#                                Global / follows COVERAGE_MIN_TOTAL; per-pkg
#                                / follows COVERAGE_PKG_FLOORS when listed.
#   - shields.io badge URL      (when COVERAGE_BADGE_OUT is set; static URL
#                                that the README can point to without an
#                                external coverage service)
#
# The script parses the standard `go test -coverprofile=...` output without
# requiring `go tool cover`, so it runs identically inside CI, act, and a
# developer's machine.
#
# Thresholds (override via env):
#   COVERAGE_MIN_TOTAL              global floor (default: 60)
#   COVERAGE_PKG_FLOORS             comma-separated pkg:pct overrides,
#                                   e.g. "config:70,event:70,eventschema:85,metrics:70"
#   COVERAGE_FAIL_ON_MISSING_FILE   "true" → fail if coverage.out is missing (default: true)
#   COVERAGE_OUT                    path to coverage profile (default: coverage.out)
#   COVERAGE_SUMMARY_OUT            optional path to write the plain-text summary
#   COVERAGE_MD_OUT                 optional path to write a markdown summary
#   COVERAGE_BADGE_OUT              optional path to write the shields.io URL
#   COVERAGE_OMIT_PROTOBUF          "true" → exclude internal/grpc/pcmiv1/*.pb.go
#                                   from stmt totals (generated protobuf only)
#   COVERAGE_ENDPOINT_OUT           optional path to write a shields.io endpoint
#                                   JSON file (committed back to main → renders
#                                   a fully dynamic badge in the README)
#   COVERAGE_RUN_URL                optional URL to the workflow run (included in
#                                   the markdown comment for traceability)
#   COVERAGE_PREVIOUS_PCT           optional previous global coverage % (shows a
#                                   delta line in the markdown — "change since
#                                   last time")
#
# Exit codes:
#   0   all thresholds met
#   1   one or more thresholds not met
#   2   invalid invocation / coverage file missing

set -euo pipefail

# Float compare: exit 0 if a >= b
ge() { awk -v a="$1" -v b="$2" 'BEGIN { exit !(a + 0 >= b + 0) }'; }

COVERAGE_OUT="${COVERAGE_OUT:-coverage.out}"
COVERAGE_MIN_TOTAL="${COVERAGE_MIN_TOTAL:-60}"
COVERAGE_PKG_FLOORS="${COVERAGE_PKG_FLOORS:-config:70,event:70,eventschema:85,graph:42,metrics:70,handler:55,service:55,repository:55,worker:45,webhook:40,embedding:55,ratelimit:60,crypto:70}"
COVERAGE_FAIL_ON_MISSING_FILE="${COVERAGE_FAIL_ON_MISSING_FILE:-true}"
COVERAGE_SUMMARY_OUT="${COVERAGE_SUMMARY_OUT:-}"
COVERAGE_MD_OUT="${COVERAGE_MD_OUT:-}"
COVERAGE_BADGE_OUT="${COVERAGE_BADGE_OUT:-}"
COVERAGE_OMIT_PROTOBUF="${COVERAGE_OMIT_PROTOBUF:-false}"
COVERAGE_ENDPOINT_OUT="${COVERAGE_ENDPOINT_OUT:-}"
COVERAGE_RUN_URL="${COVERAGE_RUN_URL:-}"
COVERAGE_PREVIOUS_PCT="${COVERAGE_PREVIOUS_PCT:-}"

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

# Emoji semantics (aligned with CI thresholds, not a fixed 70% bar):
#   • Global title: ✅ if global % >= COVERAGE_MIN_TOTAL, else 🔴
#   • Per package: ✅ / 🔴 vs COVERAGE_PKG_FLOORS when that pkg is listed;
#     otherwise ✅ if >= COVERAGE_MIN_TOTAL, 🟡 if >= 30%, else 🔴

GREEN="✅"
YELLOW="🟡"
RED="🔴"

GLOBAL_PCT="$(printf '%s\n' "$SUMMARY" | awk -F'\t' '$1=="GLOBAL" { if ($3>0) printf "%.2f", ($4/$3)*100; else print "0.00" }')"
if ge "$GLOBAL_PCT" "$COVERAGE_MIN_TOTAL"; then
  GLOBAL_EMOJI="$GREEN"
else
  GLOBAL_EMOJI="$RED"
fi

if [ -n "$COVERAGE_MD_OUT" ]; then
  {
    echo "## ${GLOBAL_EMOJI} Test coverage: \`${GLOBAL_PCT}%\`"
    echo ""

    # ── Point 8 (PR #119): "change since last time" delta ──────────────────
    if [ -n "$COVERAGE_PREVIOUS_PCT" ] && [ "$COVERAGE_PREVIOUS_PCT" != "0" ]; then
      DELTA="$(awk -v cur="$GLOBAL_PCT" -v prev="$COVERAGE_PREVIOUS_PCT" 'BEGIN {
        d = cur - prev
        if (d >= 0) printf "+%.2f", d
        else printf "%.2f", d
      }')"
      DELTA_EMOJI="$(awk -v d="$DELTA" 'BEGIN { if (d+0 >= 0) print "📈"; else print "📉" }')"
      echo "_${DELTA_EMOJI} Δ from previous: **${DELTA}%** (was ${COVERAGE_PREVIOUS_PCT}%)_"
      echo ""
    fi

    # ── Point 2 (PR #119): workflow run link ───────────────────────────────
    if [ -n "$COVERAGE_RUN_URL" ]; then
      echo "> 🔗 [View workflow run](${COVERAGE_RUN_URL})"
      echo ""
    fi

    echo "_Global statement coverage; threshold = **${COVERAGE_MIN_TOTAL}%** (\`scripts/ci_coverage_check.sh\`)._"
    echo ""
    echo "| Package | Stmts | Covered | Coverage |"
    echo "|---|---:|---:|---:|"
    printf '%s\n' "$SUMMARY" | awk -F'\t' \
      -v floors="$COVERAGE_PKG_FLOORS" \
      -v gmin="$COVERAGE_MIN_TOTAL" \
      -v green="$GREEN" \
      -v yellow="$YELLOW" \
      -v red="$RED" '
      BEGIN {
        n = split(floors, a, ",")
        for (i = 1; i <= n; i++) {
          if (split(a[i], kv, ":") == 2) {
            gsub(/^[ \t]+|[ \t]+$/, "", kv[1])
            if (kv[1] != "" && kv[2] != "") floor[kv[1]] = kv[2] + 0
          }
        }
      }
      $1=="PKG" {
        pkg = $2
        pct = ($3 > 0) ? ($4/$3)*100 : 0
        if (pkg in floor) {
          emoji = (pct >= floor[pkg]) ? green : red
        } else {
          min = gmin + 0
          if (pct >= min) emoji = green
          else if (pct >= 30) emoji = yellow
          else emoji = red
        }
        printf "| `%s` | %d | %d | %s %.1f%% |\n", pkg, $3, $4, emoji, pct
      }
    ' | sort
    # ── Point 5 (PR #119): fixed Global row — emoji passed via -v, not
    # embedded as a literal awk string (which was a bug: "$GLOBAL_EMOJI"
    # inside single quotes never expanded). ──
    echo "| **Global** | $(printf '%s\n' "$SUMMARY" | awk -F'\t' -v ge="$GLOBAL_EMOJI" '$1=="GLOBAL" { pct = ($3 > 0) ? ($4/$3)*100 : 0; printf "**%d** | **%d** | **%s %.1f%%**", $3, $4, ge, pct }') |"
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

# ─── Dynamic badge (shields.io endpoint JSON) ────────────────────────────────
#
# When COVERAGE_ENDPOINT_OUT is set, write a JSON file in shields.io endpoint
# format. The README points at this file via:
#
#   https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/<owner>/<repo>/main/badges/coverage.json
#
# CI commits the file back to main on every push, so the badge updates without
# any external coverage service (Codecov, Coveralls, gists, etc.).
if [ -n "$COVERAGE_ENDPOINT_OUT" ]; then
  ENDPOINT_COLOR="$(awk -v p="$GLOBAL_PCT" 'BEGIN {
    p += 0
    if (p >= 90) print "brightgreen"
    else if (p >= 75) print "green"
    else if (p >= 60) print "yellowgreen"
    else if (p >= 40) print "yellow"
    else if (p >= 20) print "orange"
    else print "red"
  }')"
  ENDPOINT_PCT="$(awk -v p="$GLOBAL_PCT" 'BEGIN { printf "%.1f", p+0 }')"
  mkdir -p "$(dirname "$COVERAGE_ENDPOINT_OUT")"
  cat > "$COVERAGE_ENDPOINT_OUT" <<JSON
{
  "schemaVersion": 1,
  "label": "coverage",
  "message": "${ENDPOINT_PCT}%",
  "color": "${ENDPOINT_COLOR}",
  "cacheSeconds": 300
}
JSON
fi

# ─── Threshold enforcement ────────────────────────────────────────────────────

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
