# Changelog

All notable changes to PCMI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The "Tag" constant in `internal/version/version.go` is the source of truth for
the public API version exposed by `/v1/version` and the gRPC `Version` RPC.

## [Unreleased]

### Added — Quality & CI (PR #1)
- `scripts/ci_coverage_check.sh`: pure-bash/awk script that parses
  `coverage.out`, computes per-package + global statement coverage, and exits
  non-zero when configurable thresholds are not met. No `go tool cover`
  dependency.
- `make test-cover`, `make cover-check`, `make cover-report` Makefile targets
  wrapping the gate locally.
- New unit tests:
  - `internal/config/config_helpers_test.go` — direct coverage of the unexported
    `envOr` / `envInt` / `envBool` helpers, plus extra `Validate(...)` cases
    (admin key, encryption key, multi-error path) and `PruneInterval` /
    `ExpiryInterval` duration accessors.
  - `internal/event/schema_test.go` — locks down the public event-type
    constants (these are part of the webhook + gRPC stream contract) and
    round-trips a `UniversalEvent` through `encoding/json`, including the
    `omitempty` behaviour for `agent_id` / `correlation_id`.
- Coverage artifact uploaded by CI (`coverage.out` + `coverage-summary.txt` +
  `coverage-summary.md` + `coverage-badge.txt`) for 14 days.
- Coverage report rendered **inside every PR**:
  - written to `$GITHUB_STEP_SUMMARY` so reviewers see the per-package table
    when they click into the `go` check;
  - posted as a **sticky comment** on the PR via
    `marocchino/sticky-pull-request-comment@v2` (header `pcmi-coverage`,
    updated in-place on every push);
  - shields.io badge URL emitted in `coverage-badge.txt` and surfaced in the
    job summary, ready to be wired into the README on each release.
- README badges row (CI / Coverage / Go version / License / API version) at
  the top of `README.md`, plus an explanatory blurb that points reviewers at
  the gate definition in CI.

### Changed — Quality & CI (PR #1)
- `.github/workflows/ci.yml`: the workflow now grants `pull-requests: write`
  (needed for the sticky PR comment) and the `go` job runs the coverage gate
  after the per-function summary. Initial thresholds:
  - Global ≥ **22 %**
  - `config` ≥ 70 %, `event` ≥ 70 %, `eventschema` ≥ 85 %, `metrics` ≥ 70 %,
    `version` ≥ 80 %
  These floors are intentionally calibrated so the current `main` is green and
  any further work that drops coverage breaks the build. Subsequent PRs are
  expected to ratchet the floor up — never down.
- `docs/local-ci.md`: documented the coverage gate and the matching make
  targets in the "Day-to-day commands" table and a new "Coverage gate" section.

### Added — Dynamic coverage badge + extra tests (PR #1 follow-up)
- **Fully dynamic coverage badge** on the README. `scripts/ci_coverage_check.sh`
  now writes a shields.io endpoint JSON to `badges/coverage.json` (controlled
  by the new `COVERAGE_ENDPOINT_OUT` env var). On every push to `main`, the
  `go` CI job commits the regenerated file back to the branch with a
  `[skip ci]` marker, so the badge displayed at the top of `README.md` always
  reflects the latest measured coverage — no Codecov, Coveralls, gist or
  external service required.
- Workflow safeguards against badge-update infinite loops:
  - `on.push.paths-ignore: ['badges/**']` so the auto-commit cannot re-trigger
    the workflow;
  - `[skip ci]` token in the commit message as belt-and-braces protection;
  - `contents: write` granted only at the `go` job level (not workflow-wide).
- Coverage scope widened: `crypto`, `embedding`, `model`, `telemetry` added
  to `COVERAGE_PKGS` in `Makefile`. These packages already shipped tests but
  were not contributing to the global denominator before; folding them in
  gives the gate an honest signal.
- Additional unit tests (deterministic, no DB/Redis/network):
  - `internal/worker/distillation_env_extra_test.go` — boundary, whitespace,
    non-numeric, and out-of-range cases for the `DISTILLATION_BATCH_SIZE` /
    `DISTILLATION_CONCURRENCY` env parsers.
  - `internal/metrics/worker_unknown_test.go` — empty-label branch of
    `IncWorkerRedisEvent` (relabelled to `unknown`) and custom-label path.
  - `internal/handler/events_handler_extra_test.go` — whitespace-only filters,
    dedup, empty entries, non-string `tenant_id` payloads.
  - `internal/middleware/public_extra2_test.go` — systematic method × path
    matrix for `IsUnauthenticatedProbe`, plus negative cases.

### Notes
- No production code paths were touched in this PR — additions are
  test-only, CI-only, and documentation. Zero risk of runtime behaviour
  change; no database migrations required.
- The auto-commit step requires the workflow's default `GITHUB_TOKEN` to
  have `contents: write` on `main`. If branch protection is enabled, add the
  `pcmi-ci[bot]` author to the allowlist or use a separate deploy key —
  otherwise the badge update step will fail soft (`git push` rejected) and
  the badge will stay frozen at the last successful update.
