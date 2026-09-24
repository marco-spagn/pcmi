# `pcmi` developer CLI

`pcmi` is a single static binary that talks to a running PCMI API over HTTP. It turns
"read the docs" into "try it in a minute": seed demo data, search it, watch live
events, travel back in time, check token spend, and prove the audit trail has not
been tampered with — including **offline** verification of an audit export.

```bash
go install github.com/marco-spagn/pcmi/cmd/pcmi@latest   # or: make build-cli → bin/pcmi
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
pcmi seed
pcmi retrieve --prefix root.demo --query "what went wrong after the deploy?"
```

## Configuration

| Setting | Flag | Environment | Default |
|---------|------|-------------|---------|
| API base URL | `--url` | `PCMI_BASE_URL` | `http://localhost:8000` |
| API key | `--api-key` | `PCMI_API_KEY` (or `PCMI_API_KEY_FILE`) | — |
| Audit signing key (offline verify) | `--signing-key-file` | `AUDIT_EXPORT_SIGNING_KEY` (or `_FILE`) | — |
| Raw JSON output | `--json` | — | human tables |
| Request timeout | `--timeout` | — | `30s` |

Global flags go **before** the command (`pcmi --json retrieve …`); command flags may
appear before or after positional arguments.

## Commands

| Command | Endpoint | Notes |
|---------|----------|-------|
| `store <path> [content\|-]` | `POST /v1/memories` | Content from stdin when `-` or omitted; `--tag` (repeatable), `--meta k=v` (value parsed as JSON when possible), `--importance` |
| `get <path>` | `GET /v1/memories/{path}` | `--version N` or `--as-of RFC3339` for time travel |
| `retrieve [words…]` | `POST /v1/retrieve` | `--query/-q`, `--prefix`, `--limit`, `--as-of`, `--tag`, `--no-rerank`; alias `search` |
| `tail` | `GET /v1/events` (SSE) | `--types a,b`, `-n N` to exit after N events; Ctrl-C to stop |
| `seed` | `POST /v1/memories` × N | Built-in 12-memory demo under `--prefix` (default `root.demo`), or `--file memories.jsonl` |
| `usage` | `GET /v1/stats/usage` | `--from`, `--to` (`YYYY-MM-DD`), `--group-by day,operation,model\|none` |
| `erase <prefix>` | `POST /v1/memories/erase` | Admin. Refuses to run without `--dry-run` or `--yes`; `--reason` is recorded in the audit chain |
| `audit verify` | `GET /v1/audit/verify` | Exit code 1 when the chain is broken |
| `audit export` | `GET /v1/audit/export` | Admin. `-o FILE`, `--from-seq`, `--to-seq`, `--limit` |
| `audit verify-export <file\|->` | — (offline) | Recomputes every hash and link, the content digest, and the HMAC; `--require-signature` for CI |
| `version` | `GET /v1/health` | CLI and server versions |

Exit codes: `0` success, `1` failure or negative verdict (broken chain, invalid export), `2` usage error.

## Walkthrough

```bash
pcmi seed --prefix root.demo
pcmi retrieve --prefix root.demo "search misses synonyms"
pcmi get root.demo.product.pricing.plan               # 25 USD (current)
pcmi get root.demo.product.pricing.plan --version 1   # 20 USD (time travel)

pcmi tail &                                            # live events in another shell
pcmi store root.demo.notes.today "Rotated the staging TLS certificate" --tag ops

pcmi usage --group-by operation                        # tokens and estimated cost
pcmi erase root.demo.customers.acme --dry-run          # exact counts, nothing deleted

pcmi audit verify
pcmi audit export -o audit.jsonl
pcmi audit verify-export audit.jsonl --signing-key-file /run/secrets/audit_key --require-signature
```

## Seed file format

One JSON object per line, same shape as `POST /v1/memories` (blank lines and `#` comments ignored):

```json
{"path": "root.kb.faq.refunds", "content": "Refunds are issued within 5 business days.", "tags": ["faq"], "importance": 0.6, "metadata": {"source": "helpdesk"}}
```

## Governance smoke test

[`scripts/smoke_governance.sh`](../scripts/smoke_governance.sh) (`make smoke-governance`) drives the CLI against a running stack and checks seed/retrieve/time travel, SSE, retention policies, GDPR erasure, the audit chain (server-side, sealed export, offline verification, tamper detection), and usage metering. Optional stricter checks: `AUDIT_KEY_FILE`, `EXPECT_RERANK=1`, `EXPECT_USAGE=1`.
