## Summary

<!-- What changed and why (1–3 sentences). Link issues: Fixes # -->

## Type of change

- [ ] Bug fix
- [ ] New feature / API change
- [ ] Documentation
- [ ] Refactor / tooling / CI
- [ ] Breaking change (describe migration)

## Checklist

- [ ] `make test` (and `make lint` for Go changes)
- [ ] `make ci-like-github` or targeted job parity (`make act-lint`, `make act-test`, `make act-integration-smoke`) for CI-touching changes
- [ ] Integration tests updated if behavior/API changed (`-tags=integration` where relevant)
- [ ] [CHANGELOG.md](../CHANGELOG.md) updated under `[Unreleased]` if user-visible
- [ ] Version bump in `internal/version/version.go` + `docs/openapi.yaml` + Helm `appVersion` if releasing API version (smoke picks up `Tag` automatically)
- [ ] OpenAPI / `docs/grpc-vs-http.md` / SDK `HTTP-API.md` updated for API changes

## Test plan

<!-- How did you verify? Commands, scenarios. -->
