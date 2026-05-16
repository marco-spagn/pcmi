# Celery ↔ PCMI (HTTP)

Minimal tasks that enqueue memory **store** and **retrieve** calls against PCMI. Use any broker Redis/RabbitMQ supported by Celery; the example defaults to Redis.

## Setup

```bash
cd examples/celery
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export PCMI_API_KEY=testkey123   # or your tenant key
export PCMI_BASE_URL=http://localhost:8000
export CELERY_BROKER_URL=redis://localhost:6379/0
```

Start a worker:

```bash
celery -A pcmi_tasks worker --loglevel=info
```

From another shell:

```python
from pcmi_tasks import pcmi_store, pcmi_retrieve
pcmi_store.delay("root.celery.demo", "hello from Celery")
pcmi_retrieve.delay("root.celery", "", 5)
```

## Notes

- Tasks use idempotent HTTP POST/POST patterns from [docs/openapi.yaml](../../docs/openapi.yaml).
- For high volume, tune Celery prefetch, HTTP timeouts in `pcmi_tasks.py`, and PCMI rate limits (`RATE_LIMIT_RPM`).
