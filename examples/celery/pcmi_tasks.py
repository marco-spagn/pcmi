"""Celery tasks that call PCMI HTTP API (store / retrieve)."""

from __future__ import annotations

import os
from typing import Optional

import httpx
from celery import Celery

app = Celery(
    "pcmi_celery_example",
    broker=os.environ.get("CELERY_BROKER_URL", "redis://localhost:6379/0"),
)

_BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
_TIMEOUT = float(os.environ.get("PCMI_HTTP_TIMEOUT_SECS", "60"))


def _session_headers() -> dict[str, str]:
    key = os.environ.get("PCMI_API_KEY")
    if not key:
        raise RuntimeError("PCMI_API_KEY is required")
    return {"X-API-Key": key, "Content-Type": "application/json"}


@app.task(name="pcmi.store")
def pcmi_store(path: str, content: str, metadata: Optional[dict] = None) -> dict:
    """POST /v1/memories"""
    payload: dict = {"path": path, "content": content}
    if metadata:
        payload["metadata"] = metadata
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.post(
            f"{_BASE}/v1/memories",
            json=payload,
            headers=_session_headers(),
        )
    r.raise_for_status()
    return r.json()


@app.task(name="pcmi.retrieve")
def pcmi_retrieve(path_prefix: str, query: str = "", limit: int = 10) -> dict:
    """POST /v1/retrieve"""
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.post(
            f"{_BASE}/v1/retrieve",
            json={"path_prefix": path_prefix, "query": query, "limit": limit},
            headers=_session_headers(),
        )
    r.raise_for_status()
    return r.json()
