import json
from collections.abc import AsyncIterator
from typing import Any

import httpx

from .models import MemoryStore, MemoryRetrieve, MemoryRollback, IngestEvent


class PCMIClient:
    def __init__(self, base_url: str, api_key: str):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.client = httpx.AsyncClient(
            base_url=self.base_url,
            headers={"X-API-Key": api_key, "Content-Type": "application/json"},
        )

    async def store(
        self,
        path: str,
        content: str,
        metadata: dict | None = None,
        *,
        tags: list[str] | None = None,
        embedding_model: str | None = None,
        embedding_space: str | None = None,
        embedding: list[float] | None = None,
        source_agent_id: str | None = None,
    ):
        payload = MemoryStore(
            path=path,
            content=content,
            metadata=metadata or {},
            tags=tags,
            embedding_model=embedding_model,
            embedding_space=embedding_space,
            embedding=embedding,
            source_agent_id=source_agent_id,
        )
        resp = await self.client.post("/v1/memories", json=payload.model_dump(exclude_none=True))
        resp.raise_for_status()
        return resp.json()

    async def retrieve(
        self,
        path_prefix: str,
        query: str = "",
        limit: int = 10,
        *,
        as_of: str | None = None,
        source_agent_id: str | None = None,
        embedding_space: str | None = None,
    ):
        payload = MemoryRetrieve(
            path_prefix=path_prefix,
            query=query,
            limit=limit,
            as_of=as_of,
            source_agent_id=source_agent_id,
            embedding_space=embedding_space,
        )
        resp = await self.client.post("/v1/retrieve", json=payload.model_dump(exclude_none=True))
        resp.raise_for_status()
        return resp.json()

    async def rollback(
        self,
        path: str,
        *,
        version: int | None = None,
        as_of: str | None = None,
    ):
        payload = MemoryRollback(path=path, version=version, as_of=as_of)
        resp = await self.client.post(
            "/v1/memories/rollback", json=payload.model_dump(exclude_none=True)
        )
        resp.raise_for_status()
        return resp.json()

    async def ingest_event(
        self,
        event_type: str,
        payload: dict | None = None,
        *,
        agent_id: str | None = None,
        correlation_id: str | None = None,
    ):
        body = IngestEvent(
            event_type=event_type,
            agent_id=agent_id,
            correlation_id=correlation_id,
            payload=payload or {},
        )
        resp = await self.client.post("/v1/events", json=body.model_dump(exclude_none=True))
        resp.raise_for_status()
        return resp.json()

    async def get_memory(self, path: str, *, version: int | None = None, as_of: str | None = None):
        params: dict[str, str | int] = {}
        if version is not None:
            params["version"] = version
        if as_of:
            params["as_of"] = as_of
        resp = await self.client.get(f"/v1/memories/{path}", params=params)
        resp.raise_for_status()
        return resp.json()

    async def batch_store(self, items: list[dict]):
        resp = await self.client.post("/v1/memories/batch", json={"items": items})
        resp.raise_for_status()
        return resp.json()

    async def batch_retrieve(self, queries: list[dict]):
        resp = await self.client.post("/v1/retrieve/batch", json={"queries": queries})
        resp.raise_for_status()
        return resp.json()

    async def export_memories(self, path_prefix: str, limit: int = 500, include_embeddings: bool = False):
        resp = await self.client.post(
            "/v1/memories/export",
            json={"path_prefix": path_prefix, "limit": limit, "include_embeddings": include_embeddings},
        )
        resp.raise_for_status()
        return resp.json()

    async def import_memories(self, entries: list[dict], mode: str = "skip"):
        resp = await self.client.post("/v1/memories/import", json={"entries": entries, "mode": mode})
        resp.raise_for_status()
        return resp.json()

    async def list_tenants(self, limit: int = 100):
        resp = await self.client.get("/v1/admin/tenants", params={"limit": limit})
        resp.raise_for_status()
        return resp.json()

    async def get_history(self, path: str, limit: int = 50):
        resp = await self.client.get(
            "/v1/memories/history",
            params={"path": path, "limit": limit},
        )
        resp.raise_for_status()
        return resp.json()

    async def list_audit(self, limit: int = 50, offset: int = 0, since: str | None = None):
        params: dict[str, str | int] = {"limit": limit, "offset": offset}
        if since:
            params["since"] = since
        resp = await self.client.get("/v1/audit", params=params)
        resp.raise_for_status()
        return resp.json()

    async def list_event_schemas(self):
        resp = await self.client.get("/v1/events/schemas")
        resp.raise_for_status()
        return resp.json()

    async def summarize(self, path_prefix: str, limit: int = 20, style: str = "brief"):
        resp = await self.client.post(
            "/v1/memories/summarize",
            json={"path_prefix": path_prefix, "limit": limit, "style": style},
        )
        resp.raise_for_status()
        return resp.json()

    async def list_webhook_dead_letter(self, limit: int = 50):
        resp = await self.client.get("/v1/webhooks/dead-letter", params={"limit": limit})
        resp.raise_for_status()
        return resp.json()

    async def list_distilled(self, path_prefix: str, limit: int = 50):
        resp = await self.client.get(
            "/v1/distilled",
            params={"path_prefix": path_prefix, "limit": limit},
        )
        resp.raise_for_status()
        return resp.json()

    async def refine(self, path_prefix: str) -> dict:
        """Queue asynchronous distillation for a path prefix (worker consumes Redis event)."""
        resp = await self.client.post(
            "/v1/memories/refine",
            json={"path_prefix": path_prefix},
        )
        resp.raise_for_status()
        return resp.json()

    async def memory_lineage(self, path: str):
        resp = await self.client.get("/v1/memories/lineage", params={"path": path})
        resp.raise_for_status()
        return resp.json()

    async def distilled_lineage(self, distilled_id: int):
        resp = await self.client.get(f"/v1/distilled/{distilled_id}/lineage")
        resp.raise_for_status()
        return resp.json()

    async def create_link(
        self,
        from_path: str,
        to_path: str,
        *,
        link_type: str = "related",
        metadata: dict | None = None,
    ):
        resp = await self.client.post(
            "/v1/memories/links",
            json={
                "from_path": from_path,
                "to_path": to_path,
                "link_type": link_type,
                "metadata": metadata or {},
            },
        )
        resp.raise_for_status()
        return resp.json()

    async def list_links(self, **params):
        resp = await self.client.get("/v1/memories/links", params=params)
        resp.raise_for_status()
        return resp.json()

    async def tenant_stats(self):
        resp = await self.client.get("/v1/stats")
        resp.raise_for_status()
        return resp.json()

    async def subscribe(
        self,
        *,
        types: list[str] | None = None,
    ) -> AsyncIterator[dict[str, Any]]:
        """Stream events from GET /v1/events (SSE). Yields `{type, payload}` objects."""
        params: dict[str, str] = {}
        if types:
            params["types"] = ",".join(types)
        async with httpx.AsyncClient(
            base_url=self.base_url,
            headers={"X-API-Key": self.api_key, "Accept": "text/event-stream"},
            timeout=None,
        ) as stream_client:
            async with stream_client.stream("GET", "/v1/events", params=params) as resp:
                resp.raise_for_status()
                buffer = ""
                async for chunk in resp.aiter_text():
                    buffer += chunk
                    while "\n\n" in buffer:
                        block, buffer = buffer.split("\n\n", 1)
                        data = ""
                        for line in block.split("\n"):
                            if line.startswith("data:"):
                                data += line[5:].lstrip()
                        if not data:
                            continue
                        yield json.loads(data)
