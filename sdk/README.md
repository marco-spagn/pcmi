# PCMI SDK

Client HTTP minimi per interagire con l’API PCMI senza accoppiare agent o runtime al backend.

## Layout

- **`python/pcmi/`** — client `httpx` asincrono (`client.py`, `models.py`). Richiede `httpx`, Python 3.10+.
- **`typescript/src/client.ts`** — client `fetch` per browser/Node. Pubblicare come pacchetto se necessario (`package.json` nel repo se presente).

## Autenticazione

Impostare sempre l’header `X-API-Key` (token associato a tenant e ruolo). Le operazioni di scrittura richiedono ruolo `write` o `admin` (vedi migrazioni `api_keys`).

## Metodi principali

| Area | Metodi tipici |
|------|----------------|
| Memoria | `store`, `retrieve`, eventuali batch/export nel client |
| Eventi | `subscribe` (SSE) — stream testuali, parsing righe `event:` / `data:` |
| Distillation | `refine(path_prefix)` — accoda job lato server (Redis + worker) |
| Lineage | `memory_lineage`, `distilled_lineage` dove implementati |
| Link / stats | `create_link`, `getStats` o equivalenti |

## OpenAPI

Contratto completo e schema richieste/risposte: [`../docs/openapi.yaml`](../docs/openapi.yaml).

## Versioning

I client non fissano la versione server: in caso di breaking change, allineare i tipi ai campi documentati in OpenAPI.
