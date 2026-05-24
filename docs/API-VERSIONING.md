# PCMI API versioning and releases

This document is the **versioning policy** for the public HTTP and gRPC API. SDK package versions and Helm chart `version` are independent unless noted below.

## Source of truth

| Artifact | Role |
|----------|------|
| [`internal/version/version.go`](../internal/version/version.go) | `Tag` (e.g. `v1.48.0`) and `Semver` (e.g. `1.48.0`) — exposed on `GET /v1/health`, `GET /v1/version`, gRPC `Version`, worker logs |
| [`CHANGELOG.md`](../CHANGELOG.md) | Human-readable release notes ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)) |
| Git tag `vX.Y.Z` | Immutable release marker; triggers [`.github/workflows/release.yml`](../.github/workflows/release.yml) |

CI and local smoke **must not** hardcode API versions in workflow YAML. Use [`scripts/ci/resolve_version.sh`](../scripts/ci/resolve_version.sh) or the `pcmi-resolve-version` composite action.

## Semantic versioning (API contract)

PCMI follows [Semantic Versioning 2.0.0](https://semver.org/) for the **HTTP/gRPC contract** (`Tag` / `Semver`):

| Bump | When | Examples |
|------|------|----------|
| **MAJOR** (`X`.0.0) | Breaking change for integrators: removed or renamed routes/RPCs, incompatible request/response shapes, auth behavior change without opt-in | Removing a list field, changing error codes clients rely on |
| **MINOR** (x.`Y`.0) | Backward-compatible capability: new endpoints, new optional fields, new env flags with safe defaults | New `POST /v1/...`, optional query params |
| **PATCH** (x.y.`Z`) | Backward-compatible fixes: bug fixes, performance, docs-only release tag, internal refactors with no contract change | Handler bug fix, migration that does not change API |

**Not** governed by API `Semver`:

- Python/TypeScript/Go SDK module versions under `sdk/`
- Helm chart `version` in `deploy/helm/pcmi/Chart.yaml` (chart packaging SemVer)
- Database migration numbers (`migrations/NNN_*.sql`)

## Release checklist (every API bump)

1. Implement the feature on a branch from `main` (one feature per PR).
2. Document behavior under `[Unreleased]` in `CHANGELOG.md` (Keep a Changelog sections: Added, Changed, Deprecated, Removed, Fixed, Security).
3. In the **last commit** of the PR, bump `internal/version/version.go` and sync:
   - `docs/openapi.yaml` → `info.version` (`Semver` without `v`)
   - `deploy/helm/pcmi/Chart.yaml` → `appVersion` (`Tag` with `v`)
   - `docs/INDEX.md`, `docs/roadmap.md`, README API badge when the public version is advertised
4. After merge to `main`, maintainers tag `vX.Y.Z` matching `Tag` in `version.go`.
5. The **Release** workflow runs `git-cliff`, attaches notes to the GitHub Release, and verifies the tag matches `version.go`.

## Commit messages (for changelog generation)

Use [Conventional Commits](https://www.conventionalcommits.org/) so `git-cliff` can classify changes:

```text
feat(scope): short imperative summary
fix(scope): ...
docs(scope): ...
chore(scope): ...
test(scope): ...
infra(scope): ...
```

Scopes are optional (`handler`, `sdk`, `helm`, `PCMI-014`, etc.). Breaking changes: add `!` after the type or a `BREAKING CHANGE:` footer.

## Branch and PR conventions

| Pattern | Use |
|---------|-----|
| `feat/pcmi-NNN-short-name` | User-facing features |
| `fix/...` | Bug fixes |
| `infra/pcmi-NNN-...` | Tooling, CI, changelog, release automation |
| `chore/...` | Repo hygiene without API impact |

PR titles often include the ticket: `[PCMI-015] infra: changelog and API versioning policy (v1.48.0)`.

## What clients should rely on

- **`GET /v1/health`** and **`GET /v1/version`** return the running `version` string (`Tag`).
- OpenAPI at [`docs/openapi.yaml`](openapi.yaml) documents the REST contract for the current `Semver`.
- gRPC proto packages use `pcmi.v1`; breaking proto changes require a new package version and are rare — prefer additive fields.

## Generating changelog locally

Requires [git-cliff](https://git-cliff.org/) (`brew install git-cliff` or `cargo install git-cliff`).

```bash
make changelog-unreleased   # since last tag → stdout
make changelog-tag TAG=v1.48.0   # notes for a specific tag (dry-run)
```

Configuration: [`cliff.toml`](../cliff.toml) at the repository root.

## Related docs

- [CONTRIBUTING.md](../CONTRIBUTING.md) — PR checklist and `make` gates
- [local-ci.md](local-ci.md) — `make ci-like-github`
- [MIGRATIONS.md](MIGRATIONS.md) — schema changes (orthogonal to API SemVer)
