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
| Bootstrap tenant / admin UI | **HTTP** (`/v1/admin/*`, `GET /v1/admin/ui`) o **gRPC** `AdminService` |
| Elenco tenant/chiavi in dev (senza curl) | **`make admin-list-keys`** (CLI `cmd/pcmi-admin`) |
| Metriche Prometheus | **HTTP** `GET /metrics` o **gRPC** `MetricsService.Scrape` (vedi autenticazione sotto) |

Dettaglio RPC: [grpc-vs-http.md](grpc-vs-http.md).

### MCP (agenti in Cursor / Claude)

Server stdio **`pcmi-mcp`** (`cmd/mcp`): tool `pcmi_store`, `pcmi_retrieve`, … — vedi **[MCP.md](MCP.md)**.

```bash
make build-mcp
make test-mcp-smoke    # handshake JSON-RPC
```

---

## HTTP — operazioni base

### Autenticazione

Header su ogni richiesta (tranne `/health`, `/metrics`, `/ready`):

```http
X-API-Key: testkey123
Content-Type: application/json
```

Ruoli: `readonly` (solo lettura), `write`, `admin` (gestione tenant/chiavi).

### Admin — ciclo di vita API key (ruolo `admin`)

| Azione | HTTP |
|--------|------|
| Crea tenant | `POST /v1/admin/tenants` |
| Elenco tenant | `GET /v1/admin/tenants` |
| Crea chiave | `POST /v1/admin/api-keys` |
| Elenco chiavi | `GET /v1/admin/api-keys` |
| Rotazione | `POST /v1/admin/api-keys/{id}/rotate` → nuovo secret (una sola volta in risposta) |
| Revoca | `DELETE /v1/admin/api-keys/{id}` |

gRPC equivalente: `AdminService` (`CreateAPIKey`, `RotateAPIKey`, …). In dev, senza curl admin:

```bash
make admin-list-keys
# Filtro tenant: go run ./cmd/pcmi-admin list --tenant default
```

Test integrazione: `make test-key-lifecycle`.

### Paginazione cursor sulle liste (PCMI-014)

Gli endpoint che restituiscono liste lunghe usano **keyset pagination** (non `OFFSET`). Query comuni:

| Parametro | Descrizione |
|-----------|-------------|
| `limit` | Righe per pagina, **1–200** (default dipende dall’endpoint: es. `50` su audit/history, `100` su `GET /v1/admin/tenants`) |
| `cursor` | Stringa opaca da `next_cursor` della risposta precedente |
| `after_id` | Alias legacy: costruisce un cursor quando `cursor` è assente. **Non** usare insieme a `cursor` (400) |

Campi di risposta comuni: `limit`, `next_cursor` (vuoto se fine lista), `has_more`.

| Endpoint | Chiave array | `total` | Note |
|----------|--------------|---------|------|
| `GET /v1/audit` | `entries` | Conteggio **globale** (con filtro `since`) | `offset` restituito sempre `0` (deprecato) |
| `GET /v1/admin/tenants` | `tenants` | Conteggio **globale** tenant | `after_id` non supportato — solo `cursor` |
| `GET /v1/admin/api-keys` | `api_keys` | assente | `after_id` non supportato |
| `GET /v1/memories/history` | `entries` | Righe **in questa pagina** | `path` obbligatorio |
| `GET /v1/distilled` | `entries` | Righe **in questa pagina** | `path_prefix` obbligatorio |
| `GET /v1/distillation/policies` | `policies` | assente | |
| `GET /v1/distillation/runs` | `runs` | assente | opz. `policy_id` |
| `GET /v1/webhooks`, `GET /v1/webhooks/dead-letter` | `entries` | assente | webhook: solo `cursor` |
| `GET /v1/memories/links` | `entries` | assente | filtri `from_path`, `to_path`, `link_type` |

Esempio (audit):

```bash
curl -s "${PCMI_BASE_URL}/v1/audit?limit=10" -H "X-API-Key: ${PCMI_API_KEY}" | jq '{total, has_more, next_cursor}'
# pagina successiva:
CUR=$(curl -s "${PCMI_BASE_URL}/v1/audit?limit=10" -H "X-API-Key: ${PCMI_API_KEY}" | jq -r .next_cursor)
curl -s "${PCMI_BASE_URL}/v1/audit?limit=10&cursor=${CUR}" -H "X-API-Key: ${PCMI_API_KEY}"
```

Il smoke CI [`scripts/ci_integration_smoke.sh`](../scripts/ci_integration_smoke.sh) verifica `audit.total`, `admin/tenants.total` e la presenza di array su distilled; per la dead-letter usa `entries | length` (non `total`).

Contratto OpenAPI: [openapi.yaml](openapi.yaml). Test: `make test-pagination`.

### Metriche Prometheus (`GET /metrics`)

L’endpoint non usa `X-API-Key`. In produzione imposta **`METRICS_SCRAPE_TOKEN`** sull’API e configura Prometheus con lo stesso segreto:

| `METRICS_SCRAPE_TOKEN` | Comportamento |
|------------------------|---------------|
| non impostato | `GET /metrics` aperto; all’avvio l’API logga un **WARNING** |
| impostato | richiede `Authorization: Bearer <token>` |

```bash
# Esempio scrape con token
export METRICS_SCRAPE_TOKEN="$(openssl rand -hex 32)"
curl -s -H "Authorization: Bearer ${METRICS_SCRAPE_TOKEN}" http://localhost:8000/metrics
```

Esempio `deploy/prometheus/prometheus.yml`: `authorization.credentials` o `bearer_token` allineati al token dell’API.

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

### Idempotenza store (`X-Idempotency-Key`)

Su `POST /v1/memories` puoi inviare un UUID nel header **`X-Idempotency-Key`**. La prima risposta `200` viene memorizzata per **24 ore** per tenant+chiave; i retry restituiscono lo stesso JSON con **`X-Idempotency-Replayed: true`**. Solo le risposte di successo vengono cachate.

```bash
KEY=$(uuidgen)
curl -s -X POST "$PCMI_BASE_URL/v1/memories" \
  -H "X-API-Key: $PCMI_API_KEY" \
  -H "X-Idempotency-Key: $KEY" \
  -d '{"path":"root.demo.idem","content":"once"}'
# Ripeti la stessa richiesta → stesso body, header X-Idempotency-Replayed: true
```

Test: `make test-idempotency`.

### Dedup ingest (`DEDUP_MODE`)

Evita duplicati per **hash del contenuto** sulla versione corrente dello stesso path (PCMI-011). Precedenza: body `dedup_mode` → header **`X-Dedup-Mode`** → `tenants.settings.dedup_mode` → env **`DEDUP_MODE`**.

| Modalità | Comportamento |
|----------|----------------|
| `none` | Nessun dedup (default) |
| `skip` | `409` o risposta con `status: deduplicated`, `dedup_action: skipped` |
| `link` | Nuova versione collegata alla sorgente esistente |
| `merge` | Aggiorna la versione esistente |

```bash
export DEDUP_MODE=skip   # in .env o compose
make smoke-dedup         # curl E2E con API su :8000
```

Test: `make test-dedup`.

### Sessioni agente

Working memory legata a una sessione; promozione verso memoria a lungo termine. Vedi **[SESSIONS.md](SESSIONS.md)** e `make smoke-sessions`.

### Operazioni avanzate (HTTP)

| Azione | Metodo e path |
|--------|----------------|
| Storia versioni | `GET /v1/memories/history?path=...` |
| Rollback | `POST /v1/memories/rollback` |
| Refine (distillazione) | `POST /v1/memories/refine` |
| Importanza | `PATCH /v1/memories/{path}/importance` |
| Link tra path | `POST/GET /v1/memories/links` |
| Stats tenant | `GET /v1/stats` |
| Webhook | `POST/GET /v1/webhooks` |
| Compact path | `POST /v1/memories/compact` |
| Sessioni | `POST/GET/DELETE /v1/sessions`, `POST .../promote` |

Contratto completo: [openapi.yaml](openapi.yaml).

---

## gRPC — stesso backend

Host: `localhost:50051` (env `GRPC_PORT`). API key nel messaggio o metadata `x-api-key`.

```bash
# Health (senza chiave)
grpcurl -plaintext localhost:50051 pcmi.v1.MemoryService/Health

# Test integrazione Go (tag integration):
#   bufconn — solo Postgres migrato (:5432), server in-process, nessun :50051
#   live    — dial TCP su GRPC_HOST (:50051); serve pcmi-api in ascolto
make infra-up                  # postgres :5432, redis :6379, api :8000 + :50051
make infra-wait                # opzionale se infra-up ha già atteso /v1/ready
make test-integration-bufconn
make test-integration-live     # fallisce se :50051 non risponde (Makefile imposta GRPC_TEST_API_KEY)

# Streams Redis (miniredis in-process, senza Docker):
make test-streams-integration

# Oppure equivalente manuale:
DATABASE_URL=postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable \
  make test-integration-bufconn
GRPC_HOST=localhost:50051 GRPC_TEST_API_KEY=testkey123 \
  go test -tags=integration -count=1 ./internal/grpc -run '^TestGRPC|^TestResolveTenantIntegration$$'
```

Su **GitHub** (messaggio commit con `CI_start`), i test gRPC live girano nel job **`integration-smoke`**, non nel job `go` (dove `GRPC_TEST_API_KEY` non è impostata e i test live vengono saltati). Vedi [local-ci.md](local-ci.md) e [integration-testing.md](integration-testing.md).

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

## Test di integrazione Go (`-tags=integration`)

Richiedono Postgres (`DATABASE_URL`). I test HTTP in `internal/handler` usano miniredis in-process.

**Attenzione — SSE:** `TestIntegrationHTTP_EventStreamMemoryStored` (httptest + Fiber SSE) può **bloccare ~10 minuti** e far fallire tutto il pacchetto `handler` per timeout. `newIntegrationHTTPApp` imposta di default `PCMI_SKIP_SSE_HTTPTEST=1`; la copertura SSE reale è in `scripts/ci_integration_smoke.sh`.

```bash
export DATABASE_URL='postgres://pcmi:pcmi@127.0.0.1:5432/pcmi?sslmode=disable'
PCMI_SKIP_SSE_HTTPTEST=1 go test -tags=integration -count=1 ./internal/handler/...
```

Dettagli, sintomi e variabili: **[integration-testing.md](integration-testing.md)**.

---

## Makefile (repo root)

| Target | Descrizione |
|--------|-------------|
| `make test` | Unit test Go |
| `make lint` | golangci-lint v2 |
| `make test-integration` | Bufconn (DB migrato) + test TCP su API già avviata |
| `make test-integration-bufconn` | Solo test in-process (Postgres + migrazioni) |
| `make test-integration-live` | gRPC TCP su `GRPC_HOST` (:50051); dopo `make infra-up` o API su host |
| `make test-streams-integration` | Bus Redis Streams in `internal/event` (miniredis, senza stack) |
| `make test-integration-handler` | Test HTTP handler (`-tags=integration`); imposta `PCMI_SKIP_SSE_HTTPTEST=1` |
| `make infra-up` / `make infra-wait` | Stack Compose + attesa `/v1/ready` (:8000) |
| `make admin-list-keys` | Tabella tenant + API key da Postgres (`DATABASE_URL`; mostra prefisso hash, non la chiave in chiaro). Filtro: `go run ./cmd/pcmi-admin list --tenant default` |
| `make free-dev-ports` / `make act-preflight` | Libera `:5432` / `:6379` (compose + container `act-*`) |
| `make test-full-real` | CI host + E2E OpenAI opz. + smoke importance/sessions/dedup + MCP — [local-ci.md](local-ci.md) |
| `make smoke-importance` | Ranking retrieve con importance/decay (curl) |
| `make smoke-sessions` | Sessioni agente curl E2E |
| `make smoke-dedup` | Dedup ingest curl E2E |
| `make test-streams-integration` | Bus Redis Streams (`internal/event`) |
| `make test-circuit-breaker` | Circuit breaker embedding |
| `make test-ratelimit-integration` | Rate limit Redis distribuito |
| `make test-idempotency` | `X-Idempotency-Key` |
| `make test-key-lifecycle` | Admin rotate/revoke |
| `make test-retrieval-scoring` | Importance + decay in SQL retrieve |
| `make test-sessions-integration` | Handler sessioni (`-tags=integration`) |
| `make test-dedup` | Dedup unit + handler |
| `make build-mcp` / `make test-mcp-unit` | MCP stdio server |
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
| `REDIS_ADDR` | `localhost:6379` | Redis (eventi + rate limit) |
| `EVENT_BACKEND` | `streams` | `streams` = Redis Streams `pcmi:events`; `pubsub` = canale legacy `memory_events` |
| `API_PORT` | `8000` | HTTP |
| `GRPC_PORT` | `50051` | gRPC |
| `METRICS_SCRAPE_TOKEN` | — | Se impostato, `GET /metrics` richiede `Authorization: Bearer …` |
| `RATE_LIMIT_DISABLED` | `false` | Disabilita rate limit (dev/CI) |
| `RATE_LIMIT_BACKEND` | `memory` | `memory` = limiter in-process; `redis` = contatori condivisi tra repliche API |
| `RATE_LIMIT_RPM` / `_READONLY` / `_WRITE` / `_ADMIN` | 120 / 200 / 100 / 30 | RPM per ruolo (backend redis o memory) |
| `DEDUP_MODE` | `none` | Default dedup ingest se tenant/richiesta non specificano |
| `OPENAI_API_KEY` | — | Embedding + semantic retrieve + LLM summarize/distill |
| `DISTILLATION_MODEL` / `DISTILLATION_BATCH_SIZE` | `gpt-4o-mini` / `10` | Worker distillation |
| `PCMI_ENCRYPTION_KEY` | — | Cifratura contenuti |
| `PCMI_BASE_URL` / `PCMI_API_KEY` | — | Solo **`cmd/mcp`** (non api/worker) |

Elenco completo: `.env.example`.

### Webhook in uscita (verifica HMAC)

Alla registrazione webhook imposta un `secret`. Ogni delivery POST include:

- `X-PCMI-Timestamp` — epoch secondi (stringa)
- `X-PCMI-Signature` — `sha256={hex(HMAC-SHA256(secret, timestamp + "." + body))}`

Verifica lato consumer con tolleranza clock (~5 min). Dettaglio: [WORKERS-AND-EVENTS.md](WORKERS-AND-EVENTS.md).

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
