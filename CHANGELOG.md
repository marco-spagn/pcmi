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
- Coverage artifact uploaded by CI (`coverage.out` + `coverage-summary.txt`)
  for 14 days.

### Changed — Quality & CI (PR #1)
- `.github/workflows/ci.yml`: the `go` job now runs the coverage gate after
  printing the per-function summary. Initial thresholds:
  - Global ≥ **22 %**
  - `config` ≥ 70 %, `event` ≥ 70 %, `eventschema` ≥ 85 %, `metrics` ≥ 70 %,
    `version` ≥ 80 %
  These floors are intentionally calibrated so the current `main` is green and
  any further work that drops coverage breaks the build. Subsequent PRs are
  expected to ratchet the floor up — never down.
- `docs/local-ci.md`: documented the coverage gate and the matching make
  targets in the "Day-to-day commands" table and a new "Coverage gate" section.

### Notes
- No production code paths were touched in this PR — additions are
  test-only, CI-only, and documentation. Zero risk of runtime behaviour
  change; no database migrations required.
