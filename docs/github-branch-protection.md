# GitHub branch protection and the coverage badge

The `go` job in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) regenerates `badges/coverage.json` and, on a **push to `main` only**, commits it back with the message `chore(ci): update coverage badge [skip ci]`. This is the only commit CI writes directly to `main`.

## Why a bypass is required

When `main` is protected by a ruleset (or classic branch protection) that requires pull requests or status checks, a direct `git push origin HEAD:main` from the workflow is rejected with `GH013: Repository rule violations`. The badge step then fails even though every test passed.

## Option A — ruleset bypass for GitHub Actions (recommended)

1. **Settings → Rules → Rulesets** → open the ruleset that targets `main`.
2. Under **Bypass list**, add **GitHub Actions** (the `github-actions` app) with mode **Always allow**.
3. Keep every other rule (required reviews, required checks) unchanged for humans.

The workflow uses `github.token`; no extra secret is needed.

## Option B — `BADGE_UPDATE_TOKEN`

If the ruleset cannot list GitHub Actions as a bypass actor, create a fine-grained personal access token (or GitHub App token) that:

- is scoped to this repository only,
- has **Contents: Read and write**,
- belongs to an actor on the ruleset bypass list.

Store it as the repository secret `BADGE_UPDATE_TOKEN`. The checkout step uses `secrets.BADGE_UPDATE_TOKEN || github.token`, so the secret takes precedence when present.

## Loop prevention

Two independent guards stop the badge commit from triggering another CI run:

| Guard | Where |
|-------|-------|
| `paths-ignore: badges/**` on `push` | `on.push` in `ci.yml` |
| `[skip ci]` in the commit message | badge step in the `go` job |

The step also exits early when `badges/coverage.json` is unchanged, so most pushes produce no commit at all.
