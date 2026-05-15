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

    async def list_distilled(self, path_prefix: str, limit: int = 50):
        resp = await self.client.get(
            "/v1/distilled",
            params={"path_prefix": path_prefix, "limit": limit},
        )
        resp.raise_for_status()
        return resp.json()

    async def refine(self, path_prefix: str) -> dict:
        """Subtree distillation is asynchronous (worker); this only peeks at existing distilled rows."""
        return await self.list_distilled(path_prefix=path_prefix, limit=1)

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
