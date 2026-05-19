# Guida all’uso di PCMI

Come collegare agenti, servizi e orchestratori a **Persistent Cognitive Memory Infrastructure**.

## Prerequisiti

1. **PostgreSQL** con estensioni `ltree` e `pgvector` (vedi `docker-compose.yml`).
2. **Redis** per eventi e worker.
3. **API key** tenant (`X-API-Key` o campo `api_key` gRPC) — in dev: `testkey123` (migration `003_rbac_api_keys.sql`, ruolo `admin`).
4. Opzionale: **`OPENAI_API_KEY`** per embedding automatici, retrieve semantico e summarize LLM.

```bash
cp .env.example .env
docker compose up -d --build
curl -s http://localhost:8000/v1/health
```

## Scegliere il trasporto

```mermaid
flowchart LR
  subgraph clients [Client]
    A[Agent / App]
  end
  subgraph transports [Trasporti]
    H[HTTP REST + SSE]
    G[gRPC]
  end
  subgraph api [pcmi-api]
    F[Fiber :8000]
    GR[gRPC :50051]
  end
  A --> H --> F
  A --> G --> GR
  F --> PG[(PostgreSQL)]
  GR --> PG
  F --> R[(Redis)]
```

| Esigenza | Consigliato |
|----------|-------------|
| Browser, curl, OpenAPI | **HTTP** |
| Throughput, batch, stream retrieve/eventi | **gRPC** |
| SDK ufficiali Python/TS | **HTTP** (wrapper in `sdk/`) |
| Bootstrap tenant / metriche Prometheus | **HTTP** (`/v1/admin/*`, `/metrics`) |

Dettaglio RPC: [grpc-vs-http.md](grpc-vs-http.md).

---

## HTTP — operazioni base

### Autenticazione

Header su ogni richiesta (tranne `/health`, `/metrics`, `/ready`):

```http
X-API-Key: testkey123
Content-Type: application/json
```

Ruoli: `readonly` (solo lettura), `write`, `admin` (gestione tenant/chiavi).

### Store e retrieve

```bash
export PCMI_BASE_URL=http://localhost:8000
export PCMI_API_KEY=testkey123

# Store
curl -s -X POST "$PCMI_BASE_URL/v1/memories" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path":"root.project.note","content":"Decisione X","tags":["decision"],"embedding_model":"unspecified"}'

# Retrieve (ibrido: prefix + testo + opzionale semantica)
curl -s -X POST "$PCMI_BASE_URL/v1/retrieve" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -d '{"path_prefix":"root.project","query":"decisione","limit":10,"tags":["decision"],"tags_match":"all"}'
```

### Eventi live (SSE)

```bash
curl -sN "$PCMI_BASE_URL/v1/events" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -H "Accept: text/event-stream" \
  -d '?types=memory.stored,memory.updated'
```

### Operazioni avanzate (HTTP)

| Azione | Metodo e path |
|--------|----------------|
| Storia versioni | `GET /v1/memories/history?path=...` |
| Rollback | `POST /v1/memories/rollback` |
| Refine (distillazione) | `POST /v1/memories/refine` |
| Link tra path | `POST/GET /v1/memories/links` |
| Stats tenant | `GET /v1/stats` |
| Webhook | `POST/GET /v1/webhooks` |
| Compact path | `POST /v1/memories/compact` |

Contratto completo: [openapi.yaml](openapi.yaml).

---

## gRPC — stesso backend

Host: `localhost:50051` (env `GRPC_PORT`). API key nel messaggio o metadata `x-api-key`.

```bash
# Health (senza chiave)
grpcurl -plaintext localhost:50051 pcmi.v1.MemoryService/Health

# Test integrazione (vedi Makefile): bufconn = solo DB; live = API su GRPC_HOST
make test-integration-bufconn
make test-integration-live   # richiede API avviata

# Oppure equivalente manuale:
DATABASE_URL=postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable \
  make test-integration-bufconn
GRPC_HOST=localhost:50051 GRPC_TEST_API_KEY=testkey123 \
  go test -tags=integration -count=1 ./internal/grpc -run '^TestGRPC'
```

Esempio concettuale (Go):

```go
conn, _ := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
client := pcmiv1.NewMemoryServiceClient(conn)
_, err := client.Store(ctx, &pcmiv1.StoreRequest{
    ApiKey: "testkey123", Path: "root.grpc.demo", Content: "hello",
    EmbeddingModel: "unspecified",
})
```

RPC disponibili: store/retrieve/batch/stream, get memory, compact, refine, links, stats, eventi, webhooks, export/import, … — vedi [grpc-vs-http.md](grpc-vs-http.md).

---

## SDK ufficiali

### Python

```bash
cd sdk/python
python3 -m venv .venv && source .venv/bin/activate
pip install -e .
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
python smoke.py
```

```python
from pcmi import PCMIClient
import asyncio

async def main():
    async with PCMIClient("http://localhost:8000", "testkey123") as c:
        await c.store("root.sdk.demo", "content", tags=["demo"])
        r = await c.retrieve("root.sdk.demo", limit=5)
        print(r["total"])
        await c.refine("root.sdk")
        async for ev in c.subscribe(types=["memory.stored"]):
            print(ev["type"])
            break

asyncio.run(main())
```

### TypeScript

```bash
cd sdk/typescript
npm ci && npm run smoke
```

Usa `smoke.mts` / `npm run smoke` — **non** heredoc `tsx` su Node 23 (vedi [sdk/README.md](../sdk/README.md)).

---

## Makefile (repo root)

| Target | Descrizione |
|--------|-------------|
| `make test` | Unit test Go |
| `make lint` | golangci-lint v2 |
| `make test-integration` | Bufconn (DB migrato) + test TCP su API già avviata |
| `make test-integration-bufconn` | Solo test in-process (Postgres + migrazioni) |
| `make test-integration-live` | Solo dial `GRPC_HOST` (serve `pcmi-api`) |
| `make act-integration-smoke` | Job CI `integration-smoke`: compose PG/Redis + binari host + `ci_integration_smoke.sh` + gRPC + SDK |
| `make ci-like-github` | Parità ampia con workflow CI (`CI_start`): lint/vuln/helm, test `-race -tags=integration`, coverage gate, poi smoke |
| `make sdk-smoke` | Smoke Python + TS (API su :8000) |

---

## Worker e cosa succede dopo lo store

Dopo `POST /v1/memories`, l’API pubblica su Redis `memory.stored` / `memory.updated`. Il **worker** (`cmd/worker`):

1. Genera **embedding** se mancante e `OPENAI_API_KEY` è impostata.
2. Può **distillare** su eventi / refine.
3. **Prune** versioni chiuse vecchie, **compact** per path (API), **expiry** su `expires_at`.

Diagramma: [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md).

---

## Variabili d’ambiente principali

| Variabile | Default | Effetto |
|-----------|---------|---------|
| `DATABASE_URL` | compose | Postgres primario |
| `DATABASE_READ_URL` | — | Replica letture |
| `REDIS_ADDR` | `localhost:6379` | Event bus |
| `API_PORT` | `8000` | HTTP |
| `GRPC_PORT` | `50051` | gRPC |
| `OPENAI_API_KEY` | — | Embedding + semantic retrieve + LLM summarize |
| `PCMI_ENCRYPTION_KEY` | — | Cifratura contenuti |
| `RATE_LIMIT_DISABLED` | `false` | Disabilita rate limit (dev/CI) |

Elenco completo: `.env.example`.

---

## Percorsi consigliati (`ltree`)

- Usa prefissi gerarchici: `root.<tenant>.<project>.<topic>`.
- Un **path** identifica una “linea” di memoria; nuove versioni condividono lo stesso path.
- **Retrieve** con `path_prefix` restituisce tutto il sotto-albero.

Vedi [DATA-MODEL.md](DATA-MODEL.md).

---

## Prossimi passi

- [retrieval-pipeline.md](retrieval-pipeline.md) — come funziona il ranking.
- [failure-modes.md](failure-modes.md) — resilienza.
- [examples/README.md](../examples/README.md) — Celery / Temporal.
