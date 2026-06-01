# CI Coverage Reporting

This document explains how PCMI reports test coverage on Pull Requests
(PR #119: production-grade coverage sticky comment system).

## How it works

- Every push to `feat/**` / `feature/**` (and `pull_request` events) triggers the `go` job.
- `scripts/ci_coverage_check.sh` runs and produces `coverage-summary.md`.
- A sticky comment (header `pcmi-coverage`) is posted/updated on the associated PR
  via `marocchino/sticky-pull-request-comment@v3`.

The goal is to **never show stale coverage numbers** on long-lived PRs.

## Key features (PR #119)

| # | Feature | Description |
|---|---------|-------------|
| 1 | **Fork support** | PR resolution uses `pull_request.head.repo.owner` from the event payload — works for both same-repo and fork PRs |
| 2 | **Workflow run link** | Each comment includes a `🔗 View workflow run` link pointing to the exact CI run that produced the numbers |
| 3 | **Multiple PRs** | If multiple open PRs share the same branch head, the most recently updated one is chosen and a warning is logged |
| 4 | **Post on failed runs** | The comment is posted even when thresholds fail — the header shows 🔴 instead of ✅ for transparency |
| 5 | **Clean formatting** | The Global row is properly bolded; emojis are passed correctly to awk (no shell escaping bugs) |
| 6 | **Documentation** | This file (`docs/ci-coverage-reporting.md`) |
| 7 | **Skip on main pushes** | The Resolve + Comment steps are skipped on `main` and `release/**` pushes — no PR comment to update |
| 8 | **Change since last time** | When `COVERAGE_PREVIOUS_PCT` is set, the comment shows a delta line: `📈 Δ from previous: +2.50% (was 44.30%)` |

## Emojis in the table

| Icon | Meaning |
|------|---------|
| ✅   | Package meets its floor or global threshold |
| 🟡   | Between 30% and the threshold (warning zone) |
| 🔴   | Below 30% or specific floor |

The Global row uses the same logic against `COVERAGE_MIN_TOTAL`.

## Comment format

Example of a posted sticky comment:

```
## ✅ Test coverage: `60.50%`

_📈 Δ from previous: **+2.30%** (was 58.20%)_

> 🔗 [View workflow run](https://github.com/marco-spagn/pcmi/actions/runs/1234567890)

_Global statement coverage; threshold = **60%** (`scripts/ci_coverage_check.sh`)._

| Package | Stmts | Covered | Coverage |
|---|---:|---:|---:|
| `config` | 120 | 100 | ✅ 83.3% |
| `handler` | 450 | 300 | 🟡 66.7% |
| `webhook` | 80 | 24 | 🔴 30.0% |
| **Global** | **1200** | **726** | **✅ 60.5%** |
```

## Key design decisions

- The comment is refreshed **after every push** (not just on PR creation).
- We support forks as much as possible — PR resolution inspects the `pull_request` event payload.
- The comment includes a direct link to the workflow run that produced the numbers.
- On failed runs, we post anyway (with 🔴) — transparency over silence.
- Delta from previous coverage is shown when available (advanced diff can be enhanced later).

## Configuration

Thresholds are defined in:
- `scripts/ci/coverage_env.sh`
- Or overridden via environment variables in the workflow.

### Environment variables consumed by `ci_coverage_check.sh`

| Variable | Default | Description |
|---|---|---|
| `COVERAGE_MIN_TOTAL` | `60` | Global coverage floor (%) |
| `COVERAGE_PKG_FLOORS` | (see script) | Comma-separated `pkg:pct` overrides |
| `COVERAGE_RUN_URL` | `""` | Workflow run URL (embedded in markdown) |
| `COVERAGE_PREVIOUS_PCT` | `""` | Previous coverage % for delta display |
| `COVERAGE_MD_OUT` | `""` | Path to write markdown summary |
| `COVERAGE_OUT` | `coverage.out` | Path to Go coverage profile |
| `COVERAGE_ENDPOINT_OUT` | `""` | Path to write shields.io endpoint JSON |

## Troubleshooting

- **Comment not appearing?** Check that the branch has an open PR and the coverage job succeeded in producing `coverage-summary.md`.
- **Wrong PR?** The resolution logic prefers the first open PR matching the branch head. If multiple PRs share the head branch, a warning is logged.
- **Fork PR not getting a comment?** Verify the workflow has `pull-requests: write` permission and that the PR author has not disabled Actions.
- **Delta not showing?** `COVERAGE_PREVIOUS_PCT` must be passed to the script (future enhancement: read it from the previous sticky comment via API).

See also the comments in `.github/workflows/ci.yml` around "Resolve PR for coverage comment".
