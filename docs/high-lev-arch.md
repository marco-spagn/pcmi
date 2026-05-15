**Persistent Cognitive Memory Infrastructure (PCMI)**  
**Documento di Architettura Ufficiale – v1.0**  
**Obiettivi, Logica di Sistema, Invecchiamento dei Dati e Processo di Raffinamento**  

**Autori:** Team Senior Software Architects & AI Infrastructure Engineers  
**Data:** 03 Maggio 2026  
**Stato:** Approvato per lo sviluppo successivo  

---

### 1. Obiettivi del Sistema PCMI

PCMI non è una semplice “memoria per chatbot”.  
È **l’equivalente di Redis + Git + Vector Search + Cognitive Event Bus** per agenti AI distribuiti.

**Obiettivi primari:**
- Fornire una **memoria persistente, versionata e indipendente dal runtime** degli agenti.
- Sopravvivere a: riavvii di container, upgrade di modelli, migrazioni di framework, sostituzione di agenti.
- Consentire **continuità cognitiva** tra agenti effimeri (ephemeral agents).
- Trasformare esperienze grezze in **conoscenza distillata di ordine superiore** nel tempo.
- Garantire **auditabilità totale** e possibilità di rollback temporale.
- Essere completamente **runtime-agnostic**, LLM-agnostic e framework-agnostic.

**Principio fondante:**
> **Agents are ephemeral. Memory is persistent.**

---

### 2. Logica Fondamentale del Sistema

PCMI adotta un modello **append-only + temporal**:

- Nessun dato viene mai sovrascritto (`UPDATE` proibito sulle tabelle di memoria).
- Ogni inserimento è una nuova versione con timestamp `valid_from`.
- La versione corrente è quella con `valid_to IS NULL`.
- Il sistema supporta query “as-of” per ricostruire lo stato cognitivo in qualsiasi momento del passato.

Questa logica garantisce:
- **Immutabilità** (impossibile corrompere la storia cognitiva).
- **Temporal queries** (rollback, audit, ricostruzione storica).
- **Distribuzione** (più agenti possono leggere/scrivere simultaneamente senza conflitti).

---

### 3. Meccanismo di “Invecchiamento” dei Dati (Temporal Versioning)

L’**invecchiamento** non è una cancellazione, ma una **transizione di validità**.

#### Come funziona nel dettaglio

Quando un nuovo ricordo viene inserito con lo stesso `path`:

1. Viene creata una **nuova riga** (append-only).
2. La riga precedente (quella con `valid_to IS NULL`) viene **soft-closed** impostando `valid_to = NOW()` sulla versione precedente.
3. La nuova riga diventa la versione corrente (`valid_from = NOW()`, `valid_to = NULL`).

**Colonne chiave nello schema:**

| Colonna          | Tipo              | Ruolo nell’invecchiamento                              |
|------------------|-------------------|---------------------------------------------------------|
| `valid_from`     | TIMESTAMPTZ       | Quando questa versione diventa attiva                   |
| `valid_to`       | TIMESTAMPTZ (NULL)| Quando questa versione viene “invecchiata” / chiusa   |
| `version`        | INTEGER           | Numero di versione incrementale (per debug)            |
| `created_at`     | TIMESTAMPTZ       | Timestamp di inserimento reale                         |

**Query di esempio per “stato attuale”** (view `current_memories`):
```sql
SELECT * FROM memory_entries 
WHERE valid_to IS NULL 
  AND path <@ 'root.test';
```

**Query storica (“as-of” al 2026-04-30 20:00)**:
```sql
SELECT * FROM memory_entries 
WHERE valid_from <= '2026-04-30 20:00:00' 
  AND (valid_to IS NULL OR valid_to > '2026-04-30 20:00:00');
```

Questo meccanismo garantisce che **non si perda mai conoscenza storica** e che gli agenti possano sempre ricostruire lo stato cognitivo in qualsiasi punto temporale.

---

### 4. Processo di Raffinamento / Distillation

La **distillation** è il cuore cognitivo di PCMI.

#### Logica di raffinamento

- **Raw memories** → esperienze atomiche grezze (tool calls, ragionamenti, alert, ecc.).
- **Distilled knowledge** → conoscenza di ordine superiore aggregata, sintetizzata e generalizzata.

**Flusso completo:**

1. **Event-driven trigger**  
   Ogni `memory.stored` o job schedulato (cron) lancia un workflow di distillation.

2. **Aggregazione**  
   Il worker seleziona un sotto-albero (`root.security.*`) in un intervallo temporale.

3. **LLM-agnostic summarization** (adapter)  
   - Aggrega 100+ raw entries.  
   - Estrae pattern ricorrenti, falsi positivi, euristiche, regole generali.

4. **Salvataggio in tabella separata**  
   Viene creata una riga in `distilled_knowledge` con:
   - `path` (es. `root.security.alerts.summary`)
   - `summary`, `insights` (JSONB)
   - `confidence_score`
   - `source_entry_ids` (array di ID raw → **tracciabilità completa**)

5. **Event emission**  
   Viene emesso `knowledge.distilled` → gli agenti possono sottoscriverlo.

**Tabella distilled_knowledge** (schema):

| Colonna                | Tipo          | Scopo |
|------------------------|---------------|-------|
| `source_entry_ids`     | BIGINT[]      | Tracciabilità completa (non si perde mai la fonte) |
| `distilled_at`         | TIMESTAMPTZ   | Quando è stata generata la conoscenza distillata |
| `confidence_score`     | FLOAT         | Livello di affidabilità del raffinamento |

**Politica di raffinamento configurabile** (in `tenants.settings`):
- Soglia minima di raw entries prima di distillare.
- Frequenza di job (ogni 5 min, ogni ora, daily).
- Modelli LLM preferiti per distillation.

---

### 5. Mappatura Completa sullo Schema del Database

**Tabella principale: `memory_entries`**
- Tutto il sistema ruota attorno a questa tabella.
- È **append-only** per design.
- `ltree path` permette navigazione gerarchica e scoping.
- `valid_from / valid_to` implementa l’invecchiamento.
- `embedding` è nullable (per generazione asincrona).

**Tabella di raffinamento: `distilled_knowledge`**
- Contiene solo conoscenza di ordine superiore.
- Collegata 1:N alle raw entries tramite `source_entry_ids`.

**Eventi (`events`)**  
- Backbone per trigger di distillation e notifiche.

**View di comodo:**
- `current_memories` → stato attuale (usato internamente dal Retrieve).

**Regole di integrità:**
- Foreign key su `tenant_id` (con default tenant pre-inserito).
- Nessun `ON DELETE CASCADE` sulle raw memories (immutabilità).

---

### v1.13 — Event schemas, webhook reliability, summarization

- **Event schema registry** (`GET /v1/events/schemas`): built-in strict schemas for core PCMI event types; `POST /v1/events` rejects payloads missing required fields.
- **Webhook delivery queue**: deliveries are persisted in `webhook_deliveries` with exponential backoff; exhausted attempts move to **dead-letter** (`GET /v1/webhooks/dead-letter`).
- **Memory summarization** (`POST /v1/memories/summarize`): extractive rollup by default; LLM summary when OpenAI is configured on the API.
- **Observability**: API (`GET /v1/health`) and worker (`:8081/health`) expose pgx pool connection metrics.

---

**Conclusione del documento**

PCMI è progettato come **substrato cognitivo persistente** per sistemi AI distribuiti del futuro.  
L’invecchiamento è gestito tramite temporal versioning (valid_from/valid_to).  
Il raffinamento avviene tramite distillation asincrona con tracciabilità completa.

Il sistema è pronto per l’evoluzione verso:
- Multi-vector spaces
- Graph memory
- On-chain memory
- Meta-cognition (distillation che usa se stessa)


