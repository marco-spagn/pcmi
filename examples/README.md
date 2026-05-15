# PCMI integration examples

Reference clients that call the **HTTP API** from common orchestrators. They are not production SDKs; use [`sdk/python`](../../sdk/python) or [`sdk/typescript`](../../sdk/typescript) for typed clients when you do not need Celery/Temporal.

| Directory | Description |
|-----------|-------------|
| [celery/](celery/) | Celery tasks: store + retrieve over HTTP (`httpx`). |
| [temporal/](temporal/) | Temporal workflow + activities calling PCMI asynchronously. |

Shared environment (see each README):

- `PCMI_BASE_URL` — API base (default `http://localhost:8000`)
- `PCMI_API_KEY` — `X-API-Key` (required)

Read scaling: optional `DATABASE_READ_URL` on the API server sends SELECT-heavy work to a PostgreSQL replica; see [docs/federation-read-replicas.md](../docs/federation-read-replicas.md).
