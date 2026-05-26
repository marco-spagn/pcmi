<!--
Guidelines for AI coding agents (Cursor, Claude, Grok, Copilot, etc.) working on PCMI.
Read this + docs/INDEX.md + docs/CODEBASE.md before making changes.
-->

# PCMI Guidelines for AI Coding Agents

This document captures hard rules, conventions, and workflows specific to the PCMI codebase. Following them prevents the most common classes of mistakes when an LLM edits this project.

## Mandatory first reads

1. `docs/INDEX.md` — the single source of truth for where to find information.
2. `docs/CODEBASE.md` — package responsibilities, data flow, and many explicit conventions.
3. `CONTRIBUTING.md` — development process, testing matrix, PR checklist.

## Critical invariants (do not violate)

- **Environment variables**: Never call `os.Getenv` (or equivalent) in production code outside `internal/config`. This is enforced by `internal/config/getenv_audit_test.go`. Use `config.Get*` helpers instead.
- **HTTP route registration** (in `internal/handler/memory_handler.go`): Specific routes (`/memories/history`, batch, lineage, compact, etc.) **must** be registered before the catch-all `GET /memories/*`. Fiber wildcard matching will break otherwise.
- **Handler thickness**: Keep HTTP/gRPC handlers extremely thin. All business logic, validation, and orchestration belongs in `internal/service/*` and `internal/repository/*`.
- **API changes** require updates in **four** places minimum:
  - `docs/openapi.yaml`
  - `docs/grpc-vs-http.md` (if gRPC surface is affected)
  - `sdk/HTTP-API.md` + the Python/TypeScript/Go SDK clients
  - `CHANGELOG.md` under `[Unreleased]`
  - `internal/version/version.go` + Helm `Chart.yaml` `appVersion` when cutting a release
- **Database migrations**: New files go in `migrations/` with the next `NNN_` prefix. Always update `docker-compose.yml` and the structural tests in `internal/deploy/`. Never edit already-applied migrations.
- **Workers & events**: When touching embedding, distillation, compaction, pruning, or expiry, also consider the circuit breaker (`internal/embedding/circuit_breaker.go`), Redis Streams consumer, and the relevant smoke scripts (`make smoke-*`).
- **Testing expectations**:
  - Every PR that touches code must pass `make test` + `make lint` at minimum.
  - CI-touching or integration-heavy changes must demonstrate parity via `make ci-like-github`, `make act-*`, or `make test-full-real`.
  - For handler changes, consider `-tags=integration` and the bufconn/live gRPC tests.
- **Do not commit**: `__pycache__/`, `*.test` binaries, `.env*` (except `.env.example`), large test output dirs (`.pcmi_test_out`, `.venv_e2e`).

## Preferred workflow for non-trivial tasks

1. Use `todo_write` (with `merge: false` initially) when the task has 3+ distinct steps.
2. Explore via `read_file`, `grep`, and the GitHub tools **before** editing.
3. Make the smallest possible change that solves the stated goal.
4. Run the relevant `make` target(s) locally or via act before proposing a PR.
5. Update documentation in the same PR (never "docs later").
6. For anything touching the public contract, also update the SDK examples and smoke tests.

## Area-specific notes

- **Retrieval / importance / temporal decay**: Changes here are subtle. Always run `make smoke-importance` and `make test-retrieval-scoring`.
- **Sessions & dedup**: Use the dedicated smoke scripts (`make smoke-sessions`, `make smoke-dedup`).
- **Distillation / MCP / webhooks**: These packages historically have lower test coverage — new code here should come with unit tests.
- **Admin / RBAC / key lifecycle**: Changes must preserve the distinction between `admin` / `write` / `readonly` roles and the key rotation/revoke flows.
- **Helm / deploy**: Run `make helm-lint` + `make deploy-structural-test`. Structural tests live in `internal/deploy/`.
- **SDKs**: The official clients are thin HTTP wrappers. When the REST contract changes, the three SDKs + `sdk/HTTP-API.md` must be updated together.

## Documentation style

- Prefer Mermaid diagrams over static images when possible (they render on GitHub).
- Keep the language in `docs/` and the README consistent with the existing precise, slightly formal tone.
- Every new public endpoint or significant behavior change needs an entry in `docs/USAGE.md` and the OpenAPI spec.

## Release & versioning

- The single source of truth for the public API version is `internal/version/version.go` (`Tag` and `Semver`).
- Use conventional commits + `cliff.toml` (preview with `make changelog-unreleased`).
- Tagged releases automatically trigger container + SDK publishing.

If you are unsure about any of the above, ask before editing. The goal is to keep PCMI's unusually high bar for correctness, testability, and documentation quality.
