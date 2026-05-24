# PCMI SDK

Thin **HTTP** clients for PCMI. They do not speak gRPC; for high-throughput store/retrieve use gRPC (`proto/pcmi/v1/memory.proto`) or call REST directly.

## Layout

| Path | Runtime | Notes |
|------|---------|--------|
| `python/pcmi/` | Python 3.10+, `httpx` | Async `PCMIClient` (store, retrieve, sessions, webhooks, …) |
| `typescript/src/` | Node / browser `fetch` | `PCMIClient` class |
| `go/pcmi/` | Go 1.25+, stdlib | `pcmi.Client` |

## Transports

| Capability | gRPC | HTTP SDK |
|------------|------|----------|
| Store / retrieve / batch | Yes | Yes |
| Sessions (working memory, promote) | Yes | Yes (Go, Python, TS) |
| Compact, refine, links, stats | Yes (gRPC v1.28+) | Yes |
| SSE events, webhooks, migrate, export | Yes (gRPC v1.29+) | Yes |
| Admin tenants/keys | Yes (`AdminService`) | Yes (HTTP — SDK uses REST) |

Details: [`../docs/grpc-vs-http.md`](../docs/grpc-vs-http.md), [`../docs/USAGE.md`](../docs/USAGE.md), and [`HTTP-API.md`](HTTP-API.md).

## Authentication

Set header `X-API-Key` on every request (tenant + role). Writes need `write` or `admin`.

## Quick start

### Python

```bash
pip install -e sdk/python
```

```bash
cd sdk/python
python -m venv .venv && source .venv/bin/activate
pip install -e .
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
python smoke.py
python admin_smoke.py
```

From repo root (API on :8000):

```bash
make sdk-smoke
```

### TypeScript

```bash
cd sdk/typescript
npm install
npm install -D typescript tsx
npm run build   # optional; smoke uses source via tsx
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
npm run smoke        # HTTP store/retrieve/compact
npm run admin-smoke  # admin list tenants/keys (read-only)
```

### Go

```bash
cd sdk/go
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
go test -race ./...
go run ./examples/basic
```

From repo root:

```bash
make sdk-go-test
make sdk-go-smoke   # requires Docker infra
make sdk-all        # Python/TS smoke + Go tests
```

```go
import "github.com/marco-spagn/pcmi/sdk/go/pcmi"

client, _ := pcmi.NewClient("http://localhost:8000", "your-key")
resp, _ := client.Store(ctx, "root.demo", "hello", nil, &pcmi.StoreOptions{Tags: []string{"sdk"}})
```

Do **not** use `npx tsx <<'HEREDOC'` for imports: Node treats stdin as `[eval]` and named exports from `.ts` fail. Use `npm run smoke` or `npx tsx smoke.mts`.

```typescript
import { PCMIClient } from "./src/client.ts";

const client = new PCMIClient("http://localhost:8000", "your-key");
await client.store("root.demo", "hello", {}, { tags: ["sdk"] });
const result = await client.retrieve("root.demo", "", 5);
```

## List pagination (HTTP)

Paginated `GET` routes accept `limit`, `cursor`, and (where supported) `after_id`. Responses include `next_cursor` and `has_more`. Some lists also return `total` (full count on audit and admin tenants; page row count on history/distilled). Pass `next_cursor` back as `?cursor=…` on the next request. The Go admin client exposes `cursor` on `ListTenants` / `ListAPIKeys`; Python/TypeScript smokes read `total` where the API still provides it.

See [USAGE.md](../docs/USAGE.md#paginazione-cursor-sulle-liste-pcmi-014) and [openapi.yaml](../docs/openapi.yaml).

## OpenAPI

Full schemas: [`../docs/openapi.yaml`](../docs/openapi.yaml).

## Versioning

SDK package versions are independent of the server API tag (`v1.x.y` on `/v1/health`). Align request bodies with OpenAPI when upgrading servers.
