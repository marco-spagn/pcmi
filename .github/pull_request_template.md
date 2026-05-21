## Summary

<!-- What changed **and why** (scope, problem → approach). Mention related PRs/branches when picking up stalled work.
     Link issues: Fixes # -->

## Context / motivation (optional)

<!-- Why this belongs in PCMI now: user-visible impact, risk, rollout. Link design notes/RFC/issue threads. -->

## Type of change

- [ ] Bug fix
- [ ] New feature / API change
- [ ] Documentation
- [ ] Refactor / tooling / CI
- [ ] Breaking change (describe migration)

## Checklist

- [ ] `make test` (and `make lint` for Go changes)
- [ ] Integration tests updated if behavior/API changed (`-tags=integration` where relevant)
- [ ] [CHANGELOG.md](../CHANGELOG.md) updated under `[Unreleased]` if user-visible
- [ ] Version bump in `internal/version/version.go` + `docs/openapi.yaml` if releasing API version
- [ ] OpenAPI / `docs/grpc-vs-http.md` / SDK `HTTP-API.md` updated for API changes
- [ ] Commit message includes `CI_start` if full GitHub CI should run

## Test plan

<!-- How did you verify? Commands, scenarios. -->
