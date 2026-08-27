# PCMI integration examples

Reference clients that call the **HTTP API** from common orchestrators. They are not production SDKs; use [`sdk/python`](../../sdk/python) or [`sdk/typescript`](../../sdk/typescript) for typed clients when you do not need Celery/Temporal.

> **Orchestration vs backend:** Each sample wraps store/retrieve as framework tools; the **LLM decides** when to call them and with which arguments (`path`, `content`, `query`). PCMI does not understand LangChain or other orchestrators — it receives **HTTP only**.

| Directory | Description |
|-----------|-------------|
| [soc-incident-graph/](soc-incident-graph/) | Optional **SOC example** dataset (CSVs, generator, loader, scenarios) for the Cognitive Graph UI — not required for other domains. |
| [cognitive-graph-entities/](cognitive-graph-entities/) | **Design-only** tenant extraction profiles (SOC + generic) for the proposed entity graph layer. |
| [celery/](celery/) | Celery tasks: store + retrieve over HTTP (`httpx`). |
| [temporal/](temporal/) | Temporal workflow + activities calling PCMI asynchronously. |
| [langchain/](langchain/) | LangChain tools: store, retrieve, session working memory. |
| [llamaindex/](llamaindex/) | LlamaIndex `FunctionTool` wrappers for store / retrieve. |
| [autogen/](autogen/) | AutoGen AgentChat `FunctionTool` for store / retrieve. |
| [crewai/](crewai/) | CrewAI `@tool` functions for store / retrieve. |

Shared HTTP helpers for the AI framework samples: [`pcmi_http.py`](pcmi_http.py) (not a production SDK).

Shared environment (see each README):

- `PCMI_BASE_URL` — API base (default `http://localhost:8000`)
- `PCMI_API_KEY` — `X-API-Key` (required)

Read scaling: optional `DATABASE_READ_URL` on the API server sends SELECT-heavy work to a PostgreSQL replica; see [docs/federation-read-replicas.md](../docs/federation-read-replicas.md).

Local automation: [`scripts/local_smoke_orchestration.sh`](../scripts/local_smoke_orchestration.sh) — smoke store→GET (replica lag) and optional Temporal (`replica` | `temporal` | `all`). If `pip install` times out (PyPI), the script retries and skips `pip` if the venv already has the packages; or use `SKIP_TEMPORAL_VENV=1` if `temporalio` and `httpx` are already in the system Python.
