# Temporal ↔ PCMI (HTTP)

Demonstrates a **Temporal** worker with activities that call PCMI over HTTP. Workflows stay deterministic; all I/O runs in activities.

## Prerequisites

- PCMI API reachable from the worker (e.g. `docker compose` + `PCMI_BASE_URL=http://host.docker.internal:8000` on macOS).
- Temporal dev server: [https://docs.temporal.io/cli](https://docs.temporal.io/cli) — `temporal server start-dev`.

## Setup

```bash
cd examples/temporal
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export PCMI_API_KEY=testkey123
export PCMI_BASE_URL=http://localhost:8000
export TEMPORAL_ADDRESS=localhost:7233
```

Terminal A — worker:

```bash
python worker.py
```

Terminal B — start a demo workflow:

```bash
python starter.py root.temporal.demo "stored via Temporal"
```

## Layout

| File | Role |
|------|------|
| `activities.py` | `httpx` async calls to `/v1/memories` and `/v1/retrieve`. |
| `workflows.py` | `PcmiDemoWorkflow`: store then retrieve. |
| `worker.py` | Registers queue `pcmi-demo`. |
| `starter.py` | CLI to run one workflow execution. |

## Production notes

- Point workers at your Temporal Cloud or self-hosted frontend service.
- Use secrets for `PCMI_API_KEY`; retry policies for transient HTTP errors.
- Read scaling: configure `DATABASE_READ_URL` on PCMI API pods when using Postgres replicas ([docs/federation-read-replicas.md](../../docs/federation-read-replicas.md)).
