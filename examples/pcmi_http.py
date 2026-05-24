"""Shared minimal HTTP helpers for PCMI integration examples (not a production SDK)."""

from __future__ import annotations

import os
from typing import Any, Optional

import httpx

_BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
_TIMEOUT = float(os.environ.get("PCMI_HTTP_TIMEOUT_SECS", "60"))


def _headers() -> dict[str, str]:
    key = os.environ.get("PCMI_API_KEY")
    if not key:
        raise RuntimeError("PCMI_API_KEY is required")
    return {"X-API-Key": key, "Content-Type": "application/json"}


def store(
    path: str,
    content: str,
    *,
    metadata: Optional[dict[str, Any]] = None,
) -> dict[str, Any]:
    """POST /v1/memories"""
    payload: dict[str, Any] = {"path": path, "content": content}
    if metadata:
        payload["metadata"] = metadata
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.post(f"{_BASE}/v1/memories", json=payload, headers=_headers())
    r.raise_for_status()
    return r.json()


def retrieve(path_prefix: str, query: str = "", limit: int = 10) -> dict[str, Any]:
    """POST /v1/retrieve"""
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.post(
            f"{_BASE}/v1/retrieve",
            json={"path_prefix": path_prefix, "query": query, "limit": limit},
            headers=_headers(),
        )
    r.raise_for_status()
    return r.json()


def create_session(*, agent_id: str = "example-agent") -> dict[str, Any]:
    """POST /v1/sessions"""
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.post(
            f"{_BASE}/v1/sessions",
            json={"agent_id": agent_id},
            headers=_headers(),
        )
    r.raise_for_status()
    return r.json()


def session_store(session_id: str, path: str, content: str) -> None:
    """POST /v1/sessions/{id}/memories"""
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.post(
            f"{_BASE}/v1/sessions/{session_id}/memories",
            json={"path": path, "content": content},
            headers=_headers(),
        )
    r.raise_for_status()


def session_list(session_id: str, *, limit: int = 50) -> dict[str, Any]:
    """GET /v1/sessions/{id}/memories"""
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.get(
            f"{_BASE}/v1/sessions/{session_id}/memories",
            params={"limit": limit},
            headers=_headers(),
        )
    r.raise_for_status()
    return r.json()


def end_session(session_id: str) -> dict[str, Any]:
    """DELETE /v1/sessions/{id}"""
    with httpx.Client(timeout=_TIMEOUT) as client:
        r = client.delete(
            f"{_BASE}/v1/sessions/{session_id}",
            headers=_headers(),
        )
    r.raise_for_status()
    return r.json()
