# CI Coverage Reporting

This document explains how PCMI reports test coverage on Pull Requests.

## How it works

- Every push to `feat/**` / `feature/**` (and `pull_request` events) triggers the `go` job.
- `scripts/ci_coverage_check.sh` runs and produces `coverage-summary.md`.
- A sticky comment (header `pcmi-coverage`) is posted/updated on the associated PR.

The goal is to **never show stale coverage numbers** on long-lived PRs.

## Emojis in the table

| Icon | Meaning |
|------|---------|
| ✅   | Package meets its floor or global threshold |
| 🟡   | Between 30% and the threshold (warning zone) |
| 🔴   | Below 30% or specific floor |

The Global row uses the same logic against `COVERAGE_MIN_TOTAL`.

## Key design decisions

- The comment is refreshed **after every push** (not just on PR creation).
- We support forks as much as possible.
- The comment includes a direct link to the workflow run that produced the numbers.

## Configuration

Thresholds are defined in:
- `scripts/ci/coverage_env.sh`
- Or overridden via environment variables in the workflow.

## Troubleshooting

- Comment not appearing? Check that the branch has an open PR and the coverage job succeeded in producing `coverage-summary.md`.
- Wrong PR? The resolution logic prefers the first open PR matching the branch head.

See also the comments in `.github/workflows/ci.yml` around "Resolve PR for coverage comment".