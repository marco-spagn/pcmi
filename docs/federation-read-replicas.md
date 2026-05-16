# Federazione multi-tenant e read replica PostgreSQL

PCMI è **multi-tenant** su un unico database: ogni riga è legata a `tenant_id` e le policy RLS dipendono da `set_tenant_context`, impostato dal middleware dopo la validazione di `X-API-Key`.

## Obiettivo operativo

In carichi **read-heavy** (molti agenti o orchestratori che leggono memoria in parallelo), si può aggiungere una **streaming replica** PostgreSQL e instradare le query di sola lettura verso la replica, lasciando **scritture, transaction e probe di readiness** sul primario.

Variabile d’ambiente sull’API:

- **`DATABASE_URL`** — connessione al **primario** (obbligatoria).
- **`DATABASE_READ_URL`** — connessione alla **replica di lettura** (opzionale). Se assente, tutto resta sul primario.

## Cosa viene instradato

| Percorso | Pool |
|----------|------|
| `Store`, batch store, import, rollback, event ingest, admin, audit insert, webhook, link **create** | Primario |
| `Retrieve`, export query, history, stats, lineage, link **list**, `GetByPath` “current” | Replica se configurata |
| `GetHistoricalVersion` (versione / `as_of`, usata anche nel rollback) | **Primario** (coerenza con scritture) |

gRPC **Retrieve** eredita lo stesso `MemoryService` → usa la replica per le SELECT come l’HTTP.

## Limiti e buone pratiche

1. **Lag di replica**: subito dopo una `POST /v1/memories`, una `POST /v1/retrieve` sulla replica può non vedere ancora la riga. Per “read-your-writes”, ripeti la lettura sul primario (non esiste header dedicato; pattern consigliato: breve retry o accetta eventual consistency).
2. **RLS**: la replica deve ripetere le stesse policy del primario (stesso schema e ruolo applicativo); su Postgres le hot standby eseguono le stesse policy sulle righe replicate.
3. **Readiness**: `GET /v1/ready` continua a fare ping del **primario** e Redis; il mero stato della replica non blocca il probe (se la replica è giù, valuta monitoring separato o health check custom).
4. **Worker**: il processo worker usa solo il primario (`DATABASE_URL`), per evitare job di scrittura/distillazione su copie in ritardo.

## Kubernetes (schizzo)

- Service interno al primario per `DATABASE_URL`.
- Service (o URI) separato per la replica in `DATABASE_READ_URL` nel `ConfigMap` dell’API.
- Non esporre la replica come endpoint pubblico se non necessario.

## Esempi orchestrazione

Per avviare job che chiamano PCMI via HTTP da Celery o Temporal, vedi [`examples/README.md`](../examples/README.md).
