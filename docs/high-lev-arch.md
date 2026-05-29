**Persistent Cognitive Memory Infrastructure (PCMI)**  
**Official Architecture Document – v1.0**  
**Goals, System Logic, Data Aging, and Refinement Process**

**Authors:** Senior Software Architects & AI Infrastructure Engineers Team  
**Date:** 03 May 2026  
**Status:** Approved for further development

---

### 1. PCMI System Goals

PCMI is not a simple "chatbot memory".  
It is **the equivalent of Redis + Git + Vector Search + Cognitive Event Bus** for distributed AI agents.

**Primary goals:**
- Provide **persistent, versioned, runtime-independent memory** for agents.
- Survive: container restarts, model upgrades, framework migrations, agent replacement.
- Enable **cognitive continuity** across ephemeral agents.
- Transform raw experiences into **higher-order distilled knowledge** over time.
- Guarantee **full auditability** and temporal rollback capability.
- Be completely **runtime-agnostic**, LLM-agnostic, and framework-agnostic.

**Founding principle:**
> **Agents are ephemeral. Memory is persistent.**

---

### 2. Core System Logic

PCMI adopts an **append-only + temporal** model:

- No data is ever overwritten (`UPDATE` prohibited on memory tables).
- Every insert is a new version with a `valid_from` timestamp.
- The current version is the one with `valid_to IS NULL`.
- The system supports "as-of" queries to reconstruct cognitive state at any point in the past.

This logic guarantees:
- **Immutability** (impossible to corrupt the cognitive history).
- **Temporal queries** (rollback, audit, historical reconstruction).
- **Distribution** (multiple agents can read/write simultaneously without conflicts).

---

### 3. Data "Aging" Mechanism (Temporal Versioning)

**Aging** is not a deletion, but a **validity transition**.

#### How it works in detail

When a new memory is inserted with the same `path`:

1. A **new row** is created (append-only).
2. The previous row (the one with `valid_to IS NULL`) is **soft-closed** by setting `valid_to = NOW()` on the previous version.
3. The new row becomes the current version (`valid_from = NOW()`, `valid_to = NULL`).

**Key columns in the schema:**

| Column           | Type              | Role in aging                                          |
|------------------|-------------------|---------------------------------------------------------|
| `valid_from`     | TIMESTAMPTZ       | When this version becomes active                        |
| `valid_to`       | TIMESTAMPTZ (NULL)| When this version is "aged" / closed                   |
| `version`        | INTEGER           | Incremental version number (for debugging)             |
| `created_at`     | TIMESTAMPTZ       | Actual insertion timestamp                             |

**Example query for "current state"** (view `current_memories`):
```sql
SELECT * FROM memory_entries 
WHERE valid_to IS NULL 
  AND path <@ 'root.test';
```

**Historical query ("as-of" at 2026-04-30 20:00)**:
```sql
SELECT * FROM memory_entries 
WHERE valid_from <= '2026-04-30 20:00:00' 
  AND (valid_to IS NULL OR valid_to > '2026-04-30 20:00:00');
```

This mechanism ensures that **historical knowledge is never lost** and that agents can always reconstruct cognitive state at any point in time.

---

### 4. Refinement / Distillation Process

**Distillation** is the cognitive core of PCMI.

#### Refinement logic

- **Raw memories** → atomic raw experiences (tool calls, reasoning, alerts, etc.).
- **Distilled knowledge** → higher-order aggregated, synthesized, and generalized knowledge.

**Complete flow:**

1. **Event-driven trigger**  
   Every `memory.stored` or scheduled job (cron) launches a distillation workflow.

2. **Aggregation**  
   The worker selects a subtree (`root.security.*`) within a time interval.

3. **LLM-agnostic summarization** (adapter)  
   - Aggregates 100+ raw entries.  
   - Extracts recurring patterns, false positives, heuristics, general rules.

4. **Save to separate table**  
   A row is created in `distilled_knowledge` with:
   - `path` (e.g. `root.security.alerts.summary`)
   - `summary`, `insights` (JSONB)
   - `confidence_score`
   - `source_entry_ids` (array of raw IDs → **full traceability**)

5. **Event emission**  
   `knowledge.distilled` is emitted → agents can subscribe to it.

**`distilled_knowledge` table** (schema):

| Column                 | Type          | Purpose |
|------------------------|---------------|---------|
| `source_entry_ids`     | BIGINT[]      | Full traceability (source is never lost) |
| `distilled_at`         | TIMESTAMPTZ   | When the distilled knowledge was generated |
| `confidence_score`     | FLOAT         | Confidence level of the refinement |

**Configurable refinement policy** (in `tenants.settings`):
- Minimum raw entries threshold before distilling.
- Job frequency (every 5 min, every hour, daily).
- Preferred LLM models for distillation.

---

### 5. Full Mapping to the Database Schema

**Main table: `memory_entries`**
- The entire system revolves around this table.
- It is **append-only** by design.
- `ltree path` enables hierarchical navigation and scoping.
- `valid_from / valid_to` implements aging.
- `embedding` is nullable (for async generation).

**Refinement table: `distilled_knowledge`**
- Contains only higher-order knowledge.
- Linked 1:N to raw entries via `source_entry_ids`.

**Events (`events`)**  
- Backbone for distillation triggers and notifications.

**Convenience views:**
- `current_memories` → current state (used internally by Retrieve).

**Integrity rules:**
- Foreign key on `tenant_id` (with pre-inserted default tenant).
- No `ON DELETE CASCADE` on raw memories (immutability).

---

### v1.13 — Event schemas, webhook reliability, summarization

- **Event schema registry** (`GET /v1/events/schemas`): built-in strict schemas for core PCMI event types; `POST /v1/events` rejects payloads missing required fields.
- **Webhook delivery queue**: deliveries are persisted in `webhook_deliveries` with exponential backoff; exhausted attempts move to **dead-letter** (`GET /v1/webhooks/dead-letter`).
- **Memory summarization** (`POST /v1/memories/summarize`): extractive rollup by default; LLM summary when OpenAI is configured on the API.
- **Observability**: API (`GET /v1/health`) and worker (`:8081/health`) expose pgx pool connection metrics.

---

**Document conclusion**

PCMI is designed as a **persistent cognitive substrate** for future distributed AI systems.  
Aging is managed via temporal versioning (valid_from/valid_to).  
Refinement occurs via async distillation with full traceability.

The system is ready to evolve toward:
- Multi-vector spaces
- Graph memory
- On-chain memory
- Meta-cognition (distillation that uses itself)
