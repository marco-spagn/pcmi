# PCMI SDK

Thin **HTTP** clients for PCMI. They do not speak gRPC; for high-throughput store/retrieve use gRPC (`proto/pcmi/v1/memory.proto`) or call REST directly.

## Layout

| Path | Runtime | Notes |
|------|---------|--------|
| `python/pcmi/` | Python 3.10+, `httpx` | Async `PCMIClient` |
| `typescript/src/` | Node / browser `fetch` | `PCMIClient` class |

## Transports

| Capability | gRPC | HTTP SDK |
|------------|------|----------|
| Store / retrieve / batch | Yes | Yes |
| Compact, refine, links, stats | No | Yes |
| SSE events, webhooks, admin | No | Yes (admin CRUD: HTTP/OpenAPI only) |

Details: [`../docs/grpc-vs-http.md`](../docs/grpc-vs-http.md) and [`HTTP-API.md`](HTTP-API.md).

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

Do **not** use `npx tsx <<'HEREDOC'` for imports: Node treats stdin as `[eval]` and named exports from `.ts` fail. Use `npm run smoke` or `npx tsx smoke.mts`.

```typescript
import { PCMIClient } from "./src/client.ts";

const client = new PCMIClient("http://localhost:8000", "your-key");
await client.store("root.demo", "hello", {}, { tags: ["sdk"] });
const result = await client.retrieve("root.demo", "", 5);
```

## OpenAPI

Full schemas: [`../docs/openapi.yaml`](../docs/openapi.yaml).

## Versioning

SDK package versions are independent of the server API tag (`v1.x.y` on `/v1/health`). Align request bodies with OpenAPI when upgrading servers.
