# PCMI Security Policy

PCMI stores agent memory at rest and exposes it via HTTP + gRPC. A
compromise of any production deployment can leak tenant memories, API
keys, or distilled knowledge — so security reports are taken seriously
and handled out-of-band.

## Reporting a vulnerability

**Please do NOT open a public GitHub issue for security bugs.**

Use this private channel:

**GitHub Private Vulnerability Reporting**:
https://github.com/marco-spagn/pcmi/security/advisories/new

Include in your report, when possible:

- the PCMI version (`/v1/health` → `version`, or
  `internal/version/version.go:Tag`)
- the deployment topology (Docker compose, Helm, bare metal, ...)
- a minimal proof-of-concept or repro steps
- any mitigations you have already applied locally

## Triage & disclosure

| Step | Target SLA |
|---|---|
| Acknowledgement of receipt | 2 business days |
| Initial assessment (severity, scope) | 5 business days |
| Coordinated fix on a private branch | 30 days (CVSS ≥ 7) / 60 days (CVSS < 7) |
| Public advisory + patched release | Same day as the fix lands on `main` |

If a reproducible exploit is already public, we accelerate the patch
release and skip the embargo window.


## Scope

In scope (we welcome reports):

- PCMI API (Fiber HTTP, gRPC `MemoryService`) — RCE, SSRF, IDOR, tenant
  isolation bypass, RLS bypass, RBAC bypass, rate-limit bypass.
- Worker (distillation, embedding, prune, expiry) — secret exfiltration,
  LLM prompt-injection that escalates beyond a single tenant, replay or
  amplification against OpenAI / external services.
- SDKs (`sdk/python`, `sdk/typescript`) — signature verification gaps in
  webhook helpers, TLS bypasses, secret logging.
- Build / supply chain — typosquats, malicious deps, lockfile tampering.



## Existing security signals in CI

PCMI runs three complementary scanners on every push to `main` and on
every PR:

| Scanner | What it checks | Workflow |
|---|---|---|
| **govulncheck** | Go module CVEs against the symbol graph | `.github/workflows/ci.yml` → `security` job |
| **Trivy** | Container image OS/library vulns (`pcmi-api`, `pcmi-worker`) | `ci.yml` → `trivy-images` job |
| **CodeQL** | SAST over Go, Python, JavaScript/TypeScript with the `security-and-quality` query pack | `.github/workflows/codeql.yml` |

Findings land in the repository's
`Security → Code scanning alerts` and `Security → Dependabot alerts`
tabs. Maintainers triage on a weekly cadence.


