# PCMI Product Audit

> **Audit esterno indipendente** — prospettiva Principal Engineer (AI infra / distributed systems / developer platforms).
> Data: 2026-07-22 · Commit base: `feat/extraction-phase-b` @ `4474cce` · API `v1.51.0`.
> Metodo: analisi statica del codice reale (non del README). Ogni claim è verificato su sorgente.
> Scope: memory infrastructure production-grade per agenti AI. **Escluse** feature consumer (login UI, dashboard utente, billing).

---

## Stato attuale

### Cosa esiste davvero (verificato sul codice)

| Area | Implementazione reale | Evidenza |
|------|----------------------|----------|
| **Core service** | API Fiber (HTTP `:8000`) + gRPC (`:50051`) + Worker (`:8081`) | `cmd/api`, `cmd/worker`, `cmd/mcp`, `cmd/migrate`, `cmd/pcmi-admin` |
| **Storage** | Postgres 16 + `pgvector` + `ltree`; Apache AGE opzionale (profilo `graph`, `:5433`) | `docker-compose.yml`, 26 migrazioni |
| **Versioning temporale** | Append-only `valid_from`/`valid_to`, query `as_of`, rollback, history | `internal/repository`, `internal/model/rollback.go` |
| **Hybrid retrieval** | BM25 + cosine(pgvector) + importance + recency, **pesi configurabili** (`ScoringWeights`, halflife decay) | `internal/repository/retrieve_sql.go:45-105` |
| **Multi-tenancy** | `tenant_id` + Row-Level Security; 3 ruoli (admin/write/readonly) | migr. 002/003, `internal/middleware` |
| **API keys** | Tabella `api_keys`, rotazione, lifecycle, rate-limit per-ruolo | migr. 003/014, `cmd/pcmi-admin` |
| **Eventi** | Redis Streams (XADD + consumer group) o pubsub legacy; SSE `/v1/events`; event schemas | `internal/event` (759 src LOC) |
| **Webhooks** | Dispatcher con retry, **DLQ**, **filtro SSRF egress** (blocca metadata cloud/ULA/loopback) | `internal/webhook` (482 src / 1498 test) |
| **Distillation** | Worker LLM multi-provider (openai/grok/anthropic/deepseek), policy engine, summarize | `internal/worker` (1875 src) |
| **Cognitive Graph** | Memory links (5 tipi) + entity extraction (A) + `:Entity` vertices (B) + link proposals (C) + entity registry/alias + `same_as` (D) | migr. 019/022-026, `internal/graph` (988 src / 2885 test) |
| **Lifecycle** | Dedup (4 modi: none/skip/link/merge), pruning/expiry worker, importance decay, compaction, idempotency | migr. 011/015/017, `internal/worker/pruning.go` |
| **Sessions** | Working memory → promote a long-term, collision-safe | migr. 016, `docs/SESSIONS.md` |
| **Reliability** | Circuit breaker (embedding), retry (webhook/store), idempotency middleware, unique-version race guard (migr. 020) | `internal/embedding/circuit_breaker.go` |
| **Observability** | OpenTelemetry tracing reale (OTLP), 17 metriche Prometheus, audit trail | `internal/telemetry`, `cmd/api/main.go:65` |
| **Read scaling** | Routing read-replica (`DATABASE_READ_URL`) filtrato in repo/handler | `internal/handler/*.go` (`readReplica` pool) |
| **API surface** | **132 endpoint HTTP v1** + **4 servizi gRPC** (~30 RPC, incl. streaming Retrieve/Events/Scrape) + bulk (Batch/Export/Import) | `proto/pcmi/v1/*.proto`, `internal/handler` |
| **SDK** | Go (reale, 1153 LOC), TypeScript (614, su npm), Python (su PyPI) | `sdk/`, badge PyPI+npm |
| **Integrazioni** | LangChain, CrewAI, AutoGen, LlamaIndex, Celery, Temporal + **MCP server** | `examples/`, `cmd/mcp` |
| **Deploy** | Docker Compose, Helm chart (lint in CI), Dockerfile multi-stage | `deploy/`, `docker/` |

### Cosa funziona bene

- **Rigore ingegneristico sopra la media**: 35.855 LOC di test vs 27.906 di sorgente (ratio 1.28×). Race guard su versioning (migr. 020), SSRF su webhook, idempotency end-to-end, DLQ. Questo è codice scritto da chi ha già preso schiaffi in produzione.
- **Versioning `as_of` come query, non come forensics** — è il vero differenziatore. Mem0/Zep salvano "l'ultimo stato"; PCMI risponde *"cosa sapevamo alle 14:32 del 3 marzo"* con un parametro. Per finance/security/healthcare è un moat reale.
- **Poliglotta by design**: stesso contratto per uno script Python di 50 righe, un microservizio Go, un tool MCP di Cursor. Questo è il claim del README ed **è vero** (gRPC + HTTP + 3 SDK + MCP).
- **Self-hosted first**: gira sul Postgres che l'org ha già. Nessun dato lascia il VPC. Vantaggio competitivo netto vs Mem0/Zep cloud-first.
- **Retrieval con pesi configurabili e decadimento temporale** già in SQL, non bolt-on.

### Livello di maturità stimato

**Core: late-beta / early-production (7/10).** Il nucleo memoria+retrieval+eventi+multitenancy è production-credibile. Ciò che manca non è nel *core* ma nel **contorno enterprise** (identity, DR, compliance export, misurabilità del retrieval) e nella **profondità delle integrazioni** (wrapper thin da ~100 LOC).

Traduzione onesta: **un team di piattaforma AI potrebbe adottarlo oggi in un ambiente controllato; un CISO enterprise lo blocca al security review** finché non ci sono SSO, backup/DR documentato e audit export immutabile.

---

## Gap critici

Priorità: **P0** = blocca l'adozione enterprise in produzione · **P1** = importante per competere · **P2** = miglioramento.

| # | Feature mancante | Perché serve | Impatto | Complessità | Prio |
|---|------------------|-------------|---------|-------------|------|
| 1 | **SSO / OIDC / SAML** (oggi solo API key) | Nessuna org media-grande adotta un servizio senza IdP integration; è il primo item del security questionnaire | Blocca deal enterprise | M | **P0** |
| 2 | **Backup / restore / DR runbook + tooling** | "Production-grade" senza strategia di backup documentata e testata è un claim non difendibile in due diligence | Rischio perdita dati; blocca compliance | M | **P0** |
| 3 | **Retrieval eval framework + benchmark riproducibili** (LOCOMO-style, NDCG/recall\@k) | Non puoi *tunare* né *dimostrare* la qualità del retrieval; Mem0/Zep pubblicano numeri, tu no | Non provabile vs competitor; tuning cieco | M | **P0** |
| 4 | **Secrets management** (Vault/KMS; oggi chiavi ed encryption key via env) | `PCMI_ENCRYPTION_KEY` in env è un finding automatico in ogni pentest | Blocca security review | S–M | **P0** |
| 5 | **LLM reranking + query rewriting / contextual retrieval** | Lo stato dell'arte (Anthropic contextual retrieval, cross-encoder rerank) alza recall del 30-50% | Gap di qualità percepito | M | **P1** |
| 6 | **Fine-grained permissions** (per-path/namespace ACL; oggi 3 ruoli coarse) | Un tenant enterprise ha team che non devono vedersi i rami a vicenda | Limita multi-team adoption | M | **P1** |
| 7 | **Immutable / tamper-evident audit export** (hash chain, WORM, export firmato) | SOC2/ISO/DORA richiedono audit trail esportabile e non alterabile | Blocca settori regolati | M | **P1** |
| 8 | **Data retention policy per-tenant/per-namespace** (oggi solo `PRUNE_RETENTION_DAYS` globale) | GDPR "right to erasure" + retention differenziata per classe di dato | Compliance gap | S–M | **P1** |
| 9 | **Cost tracking LLM/embeddings** (token & $ per tenant/operazione) | Chi self-hosta vuole sapere quanto spende in embedding/distillation per tenant | FinOps mancante | S | **P1** |
| 10 | **Confidence score sulle memorie regolari** (oggi solo su proposals/entities) | Un memory layer AI-native dovrebbe pesare l'affidabilità di ogni fatto, non solo delle proposte | Retrieval quality | S–M | **P1** |
| 11 | **Deep framework integrations** (le attuali sono wrapper ~100 LOC) | LangGraph checkpointer nativo, Mem0-compatible adapter, LlamaIndex `VectorStore`/`BaseMemory` reali | Adoption friction | M | **P1** |
| 12 | **SCIM provisioning** | Deprovisioning automatico utenti/chiavi da IdP è requisito enterprise | Enterprise IT | M | **P2** |
| 13 | **Feedback loop dai risultati di retrieval** (click/usage → re-rank) | Chiude il ciclo di apprendimento; nessun competitor OSS lo fa bene | Differenziatore | M–L | **P2** |
| 14 | **Semantic memory merging** (oltre il dedup by-hash) | Fondere fatti equivalenti espressi diversamente, non solo hash-identici | Qualità memoria | L | **P2** |
| 15 | **Plugin/extractor registry** | Profili di estrazione e reranker come plugin versionati/condivisibili | Ecosystem | M | **P2** |
| 16 | **Multi-region active-active / sharding per tenant** | Scala oltre il singolo Postgres per tenant molto grandi o requisiti di data-residency | Scala estrema | L | **P2** |

---

## Miglioramenti architetturali

### Colli di bottiglia identificati

1. **Postgres monolitico multi-ruolo.** Un'unica istanza fa memoria + vector + graph (AGE) + stato eventi. È elegante per il self-host (il claim "gira sul tuo Postgres" è forte) ma:
   - **pgvector HNSW ha un soffitto** a cardinalità molto alta (decine di milioni di vettori/tenant) rispetto a vector DB dedicati. Serve una strategia di *tiering* (vettori caldi in pgvector, freddi altrove) o partizionamento per tenant.
   - **AGE sullo stesso engine** compete per le stesse risorse. Documentare quando separare l'istanza graph (già supportato via `:5433`).
   - *Raccomandazione*: pubblicare **capacity guidance** (righe/vettori per istanza, quando fare partition/shard) in `docs/scalability.md`, con numeri misurati.

2. **Ingest throughput dell'embedding worker.** Il backfill è a rate limitato (batch piccoli). A ingest sostenuto (migliaia di memorie/min) l'embedding diventa il collo. *Raccomandazione*: coda embedding dedicata con backpressure esplicito + metrica di lag pubblicata (`embedding_backlog_depth`) + opzione embedding sincrono per path critici.

3. **Redis Streams, singolo consumer group.** La concorrenza worker c'è, ma non c'è partizionamento per chiave (es. per tenant) → un tenant "rumoroso" può affamare gli altri. *Raccomandazione*: partition key per tenant + fairness scheduling.

4. **Nessuna cache di retrieval.** Query identiche ricalcolano il ranking. *Raccomandazione*: cache Redis dei result set con invalidazione su `memory.stored` del path_prefix.

### Verifica visione vs implementazione

Il README è **onesto e auto-consapevole** (dichiara esplicitamente cosa PCMI *non* è, e si posiziona contro Mem0/Zep/LangGraph senza overselling). L'implementazione **mantiene** le promesse core: versioning `as_of`, hybrid retrieval, multi-tenancy, eventi, self-host. Il delta visione↔realtà è nel **contorno enterprise**, non nel cuore. Raro e apprezzabile.

---

## Miglioramenti Developer Experience

Obiettivo dichiarato implicito: **integrare PCMI in < 15 minuti.** Stato attuale e gap:

| Elemento | Stato | Gap per il "<15 min" |
|----------|-------|----------------------|
| Quickstart | README "5 minuti" con curl | ✅ buono; manca **`npx create-pcmi-app`** / template repo |
| SDK Python/TS/Go | Pubblicati (PyPI/npm) | ✅; TS a 614 LOC è sottile — mancano tipi retrieval avanzati |
| CLI (`pcmi-admin`) | Thin (157 LOC: tenant/key) | ❌ Manca CLI dev-facing: `pcmi store`, `pcmi retrieve`, `pcmi tail-events`, `pcmi seed` |
| Error messages | Presenti | Verificare che siano **azionabili** (codice + hint + doc-link), non solo status |
| Local dev | Docker Compose solido | ✅; aggiungere **profilo one-command** con seed di dati demo |
| MCP server | Esiste (`cmd/mcp`) | ✅ forte per Cursor/Claude; documentare setup in 3 righe |
| Playground | Assente | ❌ Manca una **web playground** read-only per provare retrieval senza scrivere codice |

**Le 3 mosse DX ad alto impatto:**
1. **`pcmi` CLI dev-facing** (store/retrieve/tail/seed) — trasforma "leggo i docs" in "provo in 60 secondi".
2. **Template starter** (`create-pcmi-app` per TS/Python) con un agente d'esempio già cablato.
3. **Error catalog**: ogni errore ha un codice stabile (`PCMI-4xx-*`) + link a runbook.

---

## Nuove feature competitive

Posizionamento vs **Mem0** (semplice, Python-first, cloud), **Zep** (temporal KG, OSS lag dietro cloud), **LangGraph Memory** (framework-locked):

1. **`as_of` time-travel come prodotto** — nessun competitor OSS lo espone come query di prima classe. Va **messo in vetrina** con benchmark e casi finance/security. È il tuo moat.
2. **Cognitive Graph con entity resolution "light" + review queue umana** — Zep ha il KG ma senza il workflow human-in-the-loop di approvazione (Phase C/D). Differenziatore per settori regolati.
3. **Cross-framework parity reale** — stesso contratto per 6 framework + MCP. Mem0 è Python-only. Da trasformare in **adapter certificati** invece di esempi thin.
4. **Provenance + audit come feature vendibile** — "ogni fatto sa da dove viene e chi l'ha dedotto". Con audit export immutabile (gap #7) diventa un argomento di compliance.
5. **Distillation policy-driven multi-provider** — consolidamento memoria con LLM intercambiabili. Raro nell'OSS.

---

## Roadmap

### 0–30 giorni — "Sbloccare il security review" (P0)
- **SSO/OIDC** (gap #1): bearer JWT validation + OIDC discovery, mappatura claim→ruolo. Almeno un IdP (Auth0/Keycloak).
- **Secrets management** (gap #4): supporto `PCMI_ENCRYPTION_KEY` via file/KMS reference invece di env raw; doc su rotazione.
- **Backup/DR runbook** (gap #2): `docs/runbooks/backup-restore.md` + script `pcmi-admin backup|restore` (wrapper `pg_dump`/PITR) + test di restore in CI.
- **Retrieval eval scaffold** (gap #3): harness `make eval-retrieval` con dataset gold + recall\@k/NDCG, anche minimale. Prima ancora del reranking: serve *misurare*.
- **CLI dev-facing** (DX): `pcmi store/retrieve/tail/seed`.

### 30–90 giorni — "Competere sulla qualità" (P1)
- **LLM reranking + contextual retrieval** (gap #5): cross-encoder opzionale + contextual chunking; misurato contro il baseline dello scaffold eval.
- **Fine-grained permissions** (gap #6): ACL per `path_prefix` con ereditarietà ltree.
- **Immutable audit export** (gap #7): hash-chain sugli eventi audit + export firmato (JSONL + checksum).
- **Retention per-tenant/namespace** (gap #8) + **cost tracking** (gap #9): token/$ per tenant su distillation ed embedding, esposti in metriche.
- **Deep integrations** (gap #11): LangGraph checkpointer nativo + LlamaIndex `BaseMemory`/`VectorStore` reali (non wrapper).
- **Confidence score** sulle memorie regolari (gap #10).

### 90+ giorni — "Piattaforma & scala" (P2)
- **SCIM** (gap #12), **plugin registry** (gap #15).
- **Feedback loop retrieval** (gap #13) + **semantic merging** (gap #14).
- **Sharding per tenant / multi-region** (gap #16) con capacity guidance misurata.
- **Web playground** read-only per retrieval.
- **Cloud/managed offering** opzionale (senza tradire il self-host first).

---

## Le 10 feature che renderebbero PCMI leader di AI Memory Infrastructure

Ragionando come CTO di una startup che vuole battere Mem0/Zep/LangGraph Memory. Ordinate per **leva competitiva**, non per facilità.

1. **`as_of` time-travel come killer feature con benchmark pubblici.** Prendi il moat che *già hai* e rendilo il titolo. Nessun altro OSS risponde "cosa sapevamo il giorno X" come query. Casi finance/security/healthcare documentati.
2. **Retrieval eval suite pubblica + leaderboard riproducibile** (LOCOMO-style). Il momento in cui pubblichi numeri recall\@k vs Mem0/Zep, la conversazione cambia. Oggi è il tuo gap più grande *e* la tua più grande opportunità di credibilità.
3. **LLM reranking + contextual retrieval** integrati e misurati. Alza la qualità percepita al livello dello stato dell'arte e ti dà i numeri del punto 2.
4. **SSO/OIDC + fine-grained ACL + audit immutabile = "Enterprise Tier".** Il pacchetto che sblocca i deal regolati che Mem0/Zep faticano a servire in self-host.
5. **Adapter certificati per LangGraph, LlamaIndex, CrewAI, AutoGen** (non esempi thin). "Drop-in memory che qualsiasi framework usa con lo stesso contratto" diventa vero, non aspirazionale.
6. **Human-in-the-loop knowledge graph** (evolvi Phase C/D): entity resolution con review queue + provenance completa. Zep ha il KG ma non il workflow di approvazione — settori regolati lo vogliono.
7. **Cost & usage FinOps per tenant** (token, $, storage, embedding lag). Chi self-hosta deve giustificare la spesa: dagli il cruscotto.
8. **Backup/PITR/DR nativi + restore testato in CI.** Trasforma "production-grade" da claim a fatto verificabile in due diligence.
9. **`pcmi` CLI + `create-pcmi-app` + playground.** Comprimi il time-to-first-value sotto i 5 minuti reali: è il moltiplicatore di adozione OSS.
10. **Memory confidence + semantic merging + feedback loop.** Il layer diventa *intelligente*: pesa l'affidabilità, fonde fatti equivalenti, impara dai retrieval usati. Questo è il salto da "database versionato" a "memoria cognitiva" — ed è coerente col nome.

---

### Nota di metodo

Questo audit valuta il codice al commit indicato. Non copre: performance misurata sotto carico reale (servono benchmark eseguiti), qualità del retrieval su dataset gold (serve la eval suite del gap #3), né security testing attivo (serve un pentest). Sono esattamente i tre strumenti di misura che, una volta costruiti, trasformano le affermazioni di questo documento da *analisi statica* a *evidenza*.
