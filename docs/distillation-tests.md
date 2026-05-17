# PCMI Distillation Pipeline — Test Guide

This document explains how to run the end-to-end distillation tests for the
PCMI (Persistent Cognitive Memory Infrastructure) project. The tests exercise
the full path: SOC incident generation → ingest via REST API → Redis event
publication → worker consumption → LLM summarization → distilled knowledge
storage, with deterministic synthetic data.

---

## 1. Prerequisites (one-time setup)

The pipeline runs entirely in Docker, plus a Python venv on the host for the
ingest client. You need:

| Tool          | Version            | Why                                  |
| ------------- | ------------------ | ------------------------------------ |
| Docker Engine | 24+                | Postgres / Redis / API / Worker      |
| Docker Compose v2 | 2.20+          | `docker compose ...` plugin syntax   |
| Python 3      | 3.10+              | Generator + PCMI Python SDK          |
| `jq`          | any recent         | JSON parsing in shell                |
| `curl`        | any recent         | API smoke calls                      |
| `make`        | GNU make 3.81+     | targets in `Makefile`                |
| OpenAI account | model `gpt-4o-mini` access | Distillation LLM         |

### 1.1 Create `.env`

```bash
cp .env.example .env
$EDITOR .env
```

At minimum set:

```dotenv
OPENAI_API_KEY=sk-...
DISTILLATION_MODEL=gpt-4o-mini
EMBEDDING_MODEL=text-embedding-3-small
```

> ⚠️ Without `OPENAI_API_KEY` the worker logs
> `⚠️  Skipping LLM distillation (no OPENAI_API_KEY)` and the distillation
> tests **fail** because no distilled record is ever produced.

### 1.2 Build the Docker images

```bash
make distill-build
```

This builds the Go binaries (`api`, `worker`) into images. You only need to
rebuild when:

- you change Go code under `internal/`, `cmd/api/`, `cmd/worker/`;
- you change the `Dockerfile`;
- you change `DISTILLATION_BATCH_SIZE` and want it baked in (it is also
  honored at runtime via env, so usually not needed).

---

## 2. Quick start

```bash
# 1) Build images (first time only)
make distill-build

# 2) Smoke test (~1 min): 100 incidents → 10 distilled
make distill-smoke
```

If that passes you have a working pipeline. To run the full battery:

```bash
make distill-all
```

---

## 3. Available `make` targets

Run `make help` for the inline cheat sheet. Summary of distillation-related
targets:

### Infrastructure

| Target              | What it does                                                |
| ------------------- | ----------------------------------------------------------- |
| `distill-build`     | `docker compose build api worker` (no-op cache OK)          |
| `distill-up`        | bring up postgres + redis + api + worker, wait for health   |
| `distill-down`      | `docker compose down -v --remove-orphans` (drops volumes)   |
| `distill-status`    | `docker compose ps` + `GET /v1/health`                       |
| `distill-logs`      | `docker compose logs -f worker` (tail 50)                   |
| `clean-distill`     | `distill-down` + remove `.pcmi_test_out/`                    |

### Scenarios

| Target                       | Workload    | Compression | Duration |
| ---------------------------- | ----------- | ----------- | -------- |
| `distill-smoke`              | 100 → 10    | 10 : 1      | ~1 min   |
| `distill-full-coverage`      | 1000 → 100  | 10 : 1      | ~3-4 min |
| `distill-high-compression`   | 1000 → 10   | 100 : 1     | ~1.5 min |
| `distill-dedup`              | idempotency check on `source_entry_ids` | n/a | ~2 min |
| `distill-stress`             | cascade auto-trigger (intentional 429 storm) | n/a | ~1.5 min |
| `distill-all`                | runs all of the above sequentially with cooldown | — | ~15 min |

### Single-shot, fully configurable

```bash
# Defaults: NUM=1000, SEED=42, DISTILL_BATCH_SIZE=10, THROTTLE_MS=0
make distill-e2e

# Override at will:
NUM=2000 DISTILL_BATCH_SIZE=50 make distill-e2e
NUM=500  DISTILL_BATCH_SIZE=100 SEED=7  make distill-e2e
```

---

## 4. What each scenario proves

### `01_full_coverage_1000_to_100.sh` — happy path

- 1000 deterministic SOC incidents are generated (seed 42).
- Worker is **stopped during ingest** so `memory.stored` cascade does not fire.
- 100 path-prefix shards (`shard_000..shard_099`) are created.
- Worker is restarted, then exactly 100 `memory.refine.requested` events are
  published on Redis channel `memory_events`.
- Expected: `active_memories Δ=1000`, `distilled_count Δ=100`,
  `raw sources used=1000`, `coverage=100%`, **0 OpenAI 429s**.

This is the canonical "production-style bulk load + offline distillation"
pattern.

### `02_high_compression_1000_to_10.sh` — fewer, richer summaries

- Same data set, but `DISTILLATION_BATCH_SIZE=100` → each distilled record
  aggregates 100 raw memories.
- 10 shards → 10 distilled records.
- Demonstrates how raising the worker `LIMIT` changes the compression ratio
  and the qualitative depth of each summary.

### `03_quick_smoke_100_to_10.sh` — CI / pre-merge

- 100 incidents → 10 distilled. ~1 minute end-to-end. Use this in CI or for
  iterative local development.

### `04_cascade_rate_limit_stress.sh` — anti-pattern demo

- **Intentionally fails.** Runs with `--keep-worker` so every `memory.stored`
  event triggers an LLM call in parallel.
- 500 incidents × 1 LLM call each ≈ saturates OpenAI's 200k TPM bucket within
  seconds → flood of `429 Too Many Requests`.
- Educational: this is exactly the failure mode that motivated the "stop
  worker during ingest" bypass.

### `05_dedup_idempotency.sh` — exactly-once distillation

- Runs scenario 03, then republishes the same 10 refine events.
- The worker's `hasDuplicateDistillation` check compares
  `source_entry_ids` and **skips the INSERT** when the source set is the
  same.
- Asserts that `distilled_count` does not change after the second pass.

---

## 5. Environment variables you can tweak

| Variable                     | Default               | Effect                                       |
| ---------------------------- | --------------------- | -------------------------------------------- |
| `DISTILLATION_BATCH_SIZE`    | `10`                  | `LIMIT` of the worker SQL query (1..200)     |
| `OPENAI_API_KEY`             | empty                 | required for LLM distillation                |
| `DISTILLATION_MODEL`         | `gpt-4o-mini`         | OpenAI model used for summarization           |
| `EMBEDDING_MODEL`            | `text-embedding-3-small` | model used by the embedding worker        |
| `NUM`                        | `1000`                | incidents to generate (orchestrator)         |
| `SEED`                       | `42`                  | RNG seed for deterministic dataset           |
| `THROTTLE_MS`                | `0`                   | inter-batch delay during ingest (irrelevant when worker is stopped) |
| `COOLDOWN_S`                 | `65`                  | sleep between scenarios in `distill-all`     |
| `PCMI_BASE_URL`              | `http://localhost:8000` | API endpoint                               |
| `PCMI_REDIS_URL`             | `redis://localhost:6379/0` | Redis URL for the publish                |
| `TENANT_ID`                  | `a1b2c3d4-e5f6-7890-abcd-ef1234567890` | SOC test tenant UUID         |

---

## 6. Troubleshooting

### `[ERR] distilled_count: y=0 (atteso z=N)`

The worker never wrote a distilled record. Common causes:

1. **`OPENAI_API_KEY` not set in `.env`** — `docker compose logs worker | grep "Skipping LLM"` to confirm.
2. **`gpt-4o-mini` rate limit hit by a previous run** — wait 60-70s and re-run.
3. **Postgres unhealthy** — `make distill-status`.

### `0 subscribers su 'memory_events'`

The worker is not running. `make distill-status`. If it crashed,
`docker compose logs worker | tail -50` and re-build with `make distill-build`.

### `invalid input syntax for type uuid: "soc-edr-agent-01"`

You are running an old version of the generator. `git pull` and ensure the
generator builds `source_agent_id` via UUID v5 (helper `agent_uuid()` in
`scripts/generate_soc_incidents_enterprise_v2.py`).

### `value too great for base (error token is "01_full_coverage_..."`

Bash 3.2 (Apple default) doesn't support `declare -A`. The current
`run_all_scenarios.sh` uses parallel indexed arrays and works on bash 3.0+.
If you still see this, you are running an older copy.

### Distillation succeeded but only the first shard distilled

Token bucket of OpenAI saturated by a previous run. Increase `COOLDOWN_S` or
run `make distill-down && sleep 90 && make distill-all`.

---

## 7. Architecture in 90 seconds

```text
┌───────────────────────┐
│ generate_soc_…_v2.py  │   1000 deterministic SOC incidents (seed=42)
│  (PCMI Python SDK)    │   batched ingest → /v1/memories/batch
└──────────┬────────────┘
           │ POST /v1/memories/batch (batch=50, --skip-publish)
           ▼
┌───────────────────────┐         memory_entries
│  PCMI API (Go)        │ ─────►  partitioned by tenant_id (RLS)
│  fiber/Postgres+pgvec │         path: root.security.incidents.soc.shard_NNN.…
└───────────────────────┘
           ▲  (worker stopped here)
           │
           ▼  (worker restarted; redis-cli PUBLISH × NUM_REFINE_EVENTS)
┌───────────────────────┐
│  Redis pub/sub        │   channel "memory_events"
│  memory.refine.requested │  Payload: {tenant_id, path_prefix=…shard_NNN}
└──────────┬────────────┘
           │
           ▼
┌───────────────────────┐
│  Worker (Go)          │   SELECT … LIMIT DISTILLATION_BATCH_SIZE
│  Distillation Engine  │   → OpenAI gpt-4o-mini summarize
│                       │   → distilled_knowledge INSERT
└───────────────────────┘
```

---

## 8. Reproducible run (full battery)

```bash
# Prerequisites met (.env has OPENAI_API_KEY, images built)
make distill-all
```

Expected outcome:

```
ALL SCENARIOS — SUMMARY
  PASS   53s       03_quick_smoke_100_to_10.sh
  PASS   184s      01_full_coverage_1000_to_100.sh
  PASS   95s       02_high_compression_1000_to_10.sh
  PASS   140s      05_dedup_idempotency.sh
  PASS   87s       04_cascade_rate_limit_stress.sh
```

> Total wall clock ≈ 12-15 min including 4 × 65s cooldown.

---

## 9. Files of interest

| Path                                                | Purpose                                                  |
| --------------------------------------------------- | -------------------------------------------------------- |
| `scripts/generate_soc_incidents_enterprise_v2.py`   | Synthetic SOC incident generator (deterministic, sharded) |
| `scripts/run_pcmi_distillation_test.sh`             | End-to-end orchestrator (the brain)                      |
| `scripts/distillation_tests/0?_*.sh`                | Per-scenario thin wrappers                               |
| `scripts/distillation_tests/run_all_scenarios.sh`   | Run-all with cooldown + PASS/FAIL summary                |
| `internal/worker/distillation.go`                   | LIMIT configurable via `DISTILLATION_BATCH_SIZE`         |
| `docker-compose.yml`                                | Propagates `DISTILLATION_BATCH_SIZE` to worker container |
| `Makefile`                                          | `make help` for everything                               |
