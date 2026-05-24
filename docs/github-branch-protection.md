# GitHub branch protection and CI automation

`main` is protected so humans merge via reviewed PRs. One exception is intentional: CI updates the README coverage badge by committing a single file on `main`.

## Coverage badge push (CI workflow)

After tests on a push to `main`, the `go` job in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) may commit **`badges/coverage.json`** only when the shields.io endpoint JSON changed. The commit message includes **`[skip ci]`**, and workflow `on.push.paths-ignore` includes `badges/**`, so badge updates do not re-trigger the full CI pipeline.

This is a chore commit (one JSON file), not feature work. Human protection rules (reviews, required checks for PRs) stay unchanged.

## Allow the push (ruleset bypass for GitHub Actions)

If badge pushes fail with **GH013** (“protected branch”), the default `GITHUB_TOKEN` cannot push to `main`. Add a bypass for the Actions app only — do **not** weaken review requirements for people.

### Repository rulesets (recommended)

1. Open the repo on GitHub → **Settings** → **Rules** → **Rulesets** (or **Branches** → ruleset linked to `main`).
2. Edit the ruleset that targets **`main`** (e.g. `main_block`).
3. Find **Bypass list** (or **Allow specified actors to bypass**).
4. Click **Add bypass** → choose **GitHub Actions** (the app; may appear as `GitHub Actions` or related to workflow runs).
5. Save the ruleset.

Only the automation actor should be on this list. Do not add individual users or teams unless you intend to let them push without review.

### Classic branch protection

If you use legacy branch protection instead of rulesets:

1. **Settings** → **Branches** → rule for `main`.
2. Enable **Allow specified actors to bypass required pull requests** (wording may vary).
3. Add **GitHub Actions** to the bypass list.
4. Save.

## Optional: `BADGE_UPDATE_TOKEN` (PAT)

If `GITHUB_TOKEN` still cannot bypass (org policy, custom rules), use a fine-scoped PAT stored as a repository secret:

| Secret | Value |
|--------|--------|
| `BADGE_UPDATE_TOKEN` | PAT with **Contents: Read and write** on this repo |

The workflow uses `secrets.BADGE_UPDATE_TOKEN` when set, otherwise `github.token`. Grant that PAT account the same **ruleset bypass** on `main` if the token is tied to a bot user instead of the Actions app.

Create the secret: **Settings** → **Secrets and variables** → **Actions** → **New repository secret**.

## Verify

1. Merge a change that affects coverage on `main`.
2. In the CI run, open the `go` job → **Commit dynamic coverage badge** step.
3. Confirm `git push` succeeds and `badges/coverage.json` on `main` updates.

If push fails, check the step log for `GH013` and confirm the bypass actor is **GitHub Actions** (or that `BADGE_UPDATE_TOKEN` has bypass).

## Related

- [local-ci.md](local-ci.md) — what local `make ci-like-github` does and does not run
- [CONTRIBUTING.md](../CONTRIBUTING.md) — human PR workflow to `main`
