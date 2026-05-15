"""Temporal activities: HTTP calls to PCMI."""

from __future__ import annotations

import os

import httpx
from temporalio import activity

_BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
_TIMEOUT = float(os.environ.get("PCMI_HTTP_TIMEOUT_SECS", "120"))


def _headers() -> dict[str, str]:
    key = os.environ.get("PCMI_API_KEY")
    if not key:
        raise RuntimeError("PCMI_API_KEY is required")
    return {"X-API-Key": key, "Content-Type": "application/json"}


@activity.defn
async def store_memory(path: str, content: str) -> dict:
    async with httpx.AsyncClient(timeout=_TIMEOUT) as client:
        r = await client.post(
            f"{_BASE}/v1/memories",
            json={
                "path": path,
                "content": content,
                "metadata": {"source": "temporal-example"},
            },
            headers=_headers(),
        )
    r.raise_for_status()
    return r.json()


@activity.defn
async def retrieve_memory(path_prefix: str, query: str, limit: int) -> dict:
    async with httpx.AsyncClient(timeout=_TIMEOUT) as client:
        r = await client.post(
            f"{_BASE}/v1/retrieve",
            json={"path_prefix": path_prefix, "query": query, "limit": limit},
            headers=_headers(),
        )
    r.raise_for_status()
    return r.json()
