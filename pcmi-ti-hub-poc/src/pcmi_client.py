"""Async HTTP client for the PCMI memory layer.

Wraps the subset of the PCMI REST API this PoC needs. Endpoint shapes were taken
verbatim from the PCMI project (docs/openapi.yaml v1.51.0 and
docs/cognitive-graph.md), not assumed:

    POST /v1/memories        -> {"id": int, "status": str, "version": int}
    POST /v1/memories/links  -> {"id": int, "from_path", "to_path", "link_type", ...}
    POST /v1/retrieve        -> {"entries": [MemoryEntry] | null, "total": int}
    GET  /v1/graph/related   -> {"memory_id","depth","count","total","entries":[{id,link_type,depth}]}
    GET  /v1/graph/chain     -> {"from_id","to_id","connected","hops","path":[{from_id,to_id,link_type,hop}]}
    GET  /v1/graph/health    -> {"available": bool, "extension": "apache-age"}

Auth: header ``X-API-Key``. The dev seed key (migration 003) is ``testkey123``
with the ``admin`` role.

Graph link contract (learned from examples/soc-incident-graph/load_to_pcmi.py):
links are created with ``from_path``/``to_path`` set to the synthetic identity
``memory.<id>`` — the integer id PCMI returns on store — NOT the memory's real
ltree path. The AGE sync trigger parses that integer as the graph vertex id, and
``/v1/graph/related`` / ``/v1/graph/chain`` accept the same integer ids.
"""
from __future__ import annotations

import json
from collections.abc import AsyncIterator
from typing import Any

import httpx

# The five user-assignable link types enforced by the DB CHECK constraint
# (migrations/021_link_type_check.sql) and the OpenAPI enum. "duplicate" exists
# but is reserved for the dedup engine and is rejected on the public link path.
LINK_TYPES: frozenset[str] = frozenset(
    {"causal", "temporal", "contradicts", "supports", "related"}
)


class PCMIError(RuntimeError):
    """Raised when a PCMI endpoint returns an unexpected status."""

    def __init__(self, where: str, status: int, body: str):
        super().__init__(f"{where} -> HTTP {status}: {body[:400]}")
        self.where = where
        self.status = status
        self.body = body


class PCMIClient:
    """Thin async wrapper over the PCMI HTTP API."""

    def __init__(self, base_url: str, api_key: str, *, timeout: float = 30.0):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self._client = httpx.AsyncClient(
            base_url=self.base_url,
            headers={"X-API-Key": api_key, "Content-Type": "application/json"},
            timeout=timeout,
        )
        # key -> memory_id assigned by PCMI on store (the PoC's id registry).
        self.ids: dict[str, int] = {}

    async def __aenter__(self) -> "PCMIClient":
        return self

    async def __aexit__(self, *_exc: object) -> None:
        await self.close()

    async def close(self) -> None:
        await self._client.aclose()

    # ── low-level helpers ────────────────────────────────────────────────────
    async def _post(self, path: str, json: dict[str, Any]) -> dict[str, Any]:
        resp = await self._client.post(path, json=json)
        if resp.status_code not in (200, 201):
            raise PCMIError(f"POST {path}", resp.status_code, resp.text)
        return resp.json() if resp.text else {}

    async def _get(self, path: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        resp = await self._client.get(path, params=params)
        if resp.status_code != 200:
            raise PCMIError(f"GET {path}", resp.status_code, resp.text)
        return resp.json() if resp.text else {}

    # ── memory write ─────────────────────────────────────────────────────────
    async def store(
        self,
        path: str,
        content: str,
        *,
        tags: list[str] | None = None,
        metadata: dict[str, Any] | None = None,
        importance: float | None = None,
        key: str | None = None,
    ) -> dict[str, Any]:
        """Create a memory. When ``key`` is given, record key -> id in ``self.ids``."""
        payload: dict[str, Any] = {"path": path, "content": content}
        if tags:
            payload["tags"] = tags
        if metadata:
            payload["metadata"] = metadata
        if importance is not None:
            payload["importance"] = importance
        resp = await self._post("/v1/memories", payload)
        mem_id = resp.get("id")
        if key is not None and mem_id is not None:
            self.ids[key] = int(mem_id)
        return resp

    async def link(
        self,
        from_id: int,
        to_id: int,
        link_type: str = "related",
        *,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Create a typed Cognitive Graph edge between two memory ids."""
        if link_type not in LINK_TYPES:
            raise ValueError(f"link_type {link_type!r} not in {sorted(LINK_TYPES)}")
        payload: dict[str, Any] = {
            "from_path": f"memory.{from_id}",
            "to_path": f"memory.{to_id}",
            "link_type": link_type,
        }
        if metadata:
            payload["metadata"] = metadata
        return await self._post("/v1/memories/links", payload)

    # ── retrieval ────────────────────────────────────────────────────────────
    async def retrieve(
        self,
        path_prefix: str,
        query: str = "",
        limit: int = 10,
        **extra: Any,
    ) -> dict[str, Any]:
        """Hybrid retrieve. An empty ``query`` lists all under the prefix; a
        non-empty ``query`` applies a lexical (BM25) AND-filter — and semantic
        ranking too when the API has OPENAI_API_KEY configured."""
        payload: dict[str, Any] = {"path_prefix": path_prefix, "query": query, "limit": limit}
        payload.update(extra)
        return await self._post("/v1/retrieve", payload)

    async def retrieve_entries(self, path_prefix: str, query: str = "", limit: int = 10, **extra: Any) -> list[dict[str, Any]]:
        """retrieve() but always returns a list (PCMI sends ``entries: null`` on no match)."""
        resp = await self.retrieve(path_prefix, query, limit, **extra)
        return resp.get("entries") or []

    # ── cognitive graph ──────────────────────────────────────────────────────
    async def graph_related(
        self,
        memory_id: int,
        *,
        depth: int = 3,
        link_types: list[str] | str | None = None,
        limit: int = 50,
        offset: int = 0,
    ) -> dict[str, Any]:
        params: dict[str, Any] = {"memory_id": memory_id, "depth": depth, "limit": limit, "offset": offset}
        if link_types:
            params["link_types"] = link_types if isinstance(link_types, str) else ",".join(link_types)
        return await self._get("/v1/graph/related", params)

    async def graph_chain(
        self,
        from_id: int,
        to_id: int,
        *,
        link_types: list[str] | str | None = None,
        max_depth: int = 10,
    ) -> dict[str, Any]:
        params: dict[str, Any] = {"from": from_id, "to": to_id, "max_depth": max_depth}
        if link_types:
            params["link_types"] = link_types if isinstance(link_types, str) else ",".join(link_types)
        return await self._get("/v1/graph/chain", params)

    async def graph_health(self) -> dict[str, Any]:
        return await self._get("/v1/graph/health")

    async def graph_cypher(self, query: str) -> dict[str, Any]:
        """Read-only Cypher passthrough (MATCH only; write role). POST /v1/graph/cypher.
        Tenant scope is injected server-side — do not add tenant_id yourself."""
        return await self._post("/v1/graph/cypher", {"query": query})

    async def get_memory(self, path: str, *, as_of: str | None = None, version: int | None = None) -> dict[str, Any]:
        """Fetch one memory by ltree path, optionally at a point in time (`as_of`)
        or a specific `version`. GET /v1/memories/{path}."""
        params: dict[str, Any] = {}
        if as_of:
            params["as_of"] = as_of
        if version is not None:
            params["version"] = version
        return await self._get(f"/v1/memories/{path}", params or None)

    # ── LLM summarization (distillation-style) ───────────────────────────────
    async def summarize(self, path_prefix: str, *, limit: int = 20, style: str = "brief") -> dict[str, Any]:
        """LLM summary of memories under a prefix (extractive fallback when the
        API has no OPENAI_API_KEY). POST /v1/memories/summarize."""
        return await self._post(
            "/v1/memories/summarize",
            {"path_prefix": path_prefix, "limit": limit, "style": style},
        )

    async def tenant_stats(self) -> dict[str, Any]:
        return await self._get("/v1/stats")

    # ── observability & audit ────────────────────────────────────────────────
    async def list_audit(self, *, limit: int = 20) -> dict[str, Any]:
        """Per-request audit trail (method, path, status, timestamp). GET /v1/audit."""
        return await self._get("/v1/audit", {"limit": limit})

    async def subscribe_events(self, *, types: list[str] | None = None) -> AsyncIterator[dict[str, Any]]:
        """Stream PCMI events (SSE). Yields parsed event objects `{Type, Payload}`.
        Cancel the consuming task to stop the stream."""
        params: dict[str, Any] = {}
        if types:
            params["types"] = ",".join(types)
        async with self._client.stream(
            "GET", "/v1/events", params=params, headers={"Accept": "text/event-stream"}
        ) as resp:
            resp.raise_for_status()
            buffer = ""
            async for chunk in resp.aiter_text():
                buffer += chunk
                while "\n\n" in buffer:
                    block, buffer = buffer.split("\n\n", 1)
                    data = "".join(
                        line[5:].strip() for line in block.split("\n") if line.startswith("data:")
                    )
                    if data:
                        try:
                            yield json.loads(data)
                        except json.JSONDecodeError:
                            continue

    # ── agent sessions (ephemeral working memory → promote to long-term) ─────
    async def create_session(self, *, agent_id: str | None = None, metadata: dict | None = None) -> dict[str, Any]:
        body: dict[str, Any] = {}
        if agent_id:
            body["agent_id"] = agent_id
        if metadata:
            body["metadata"] = metadata
        return await self._post("/v1/sessions", body)

    async def store_session_memory(
        self, session_id: str, path: str, content: str, *, metadata: dict | None = None, tags: list[str] | None = None
    ) -> dict[str, Any]:
        body: dict[str, Any] = {"path": path, "content": content}
        if metadata:
            body["metadata"] = metadata
        if tags:
            body["tags"] = tags
        return await self._post(f"/v1/sessions/{session_id}/memories", body)

    async def list_session_memories(
        self, session_id: str, *, limit: int = 50, path_prefix: str | None = None, include_long_term: bool = False
    ) -> dict[str, Any]:
        params: dict[str, Any] = {"limit": limit}
        if path_prefix:
            params["path_prefix"] = path_prefix
        if include_long_term:
            params["include_long_term"] = "true"
        return await self._get(f"/v1/sessions/{session_id}/memories", params)

    async def promote_session(self, session_id: str, *, target_prefix: str | None = None) -> dict[str, Any]:
        body: dict[str, Any] = {}
        if target_prefix:
            body["target_prefix"] = target_prefix
        return await self._post(f"/v1/sessions/{session_id}/promote", body)

    async def end_session(self, session_id: str) -> dict[str, Any]:
        resp = await self._client.delete(f"/v1/sessions/{session_id}")
        if resp.status_code not in (200, 204):
            raise PCMIError(f"DELETE /v1/sessions/{session_id}", resp.status_code, resp.text)
        return resp.json() if resp.text else {}
