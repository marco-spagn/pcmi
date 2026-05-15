import httpx
from .models import MemoryStore, MemoryRetrieve


class PCMIClient:
    def __init__(self, base_url: str, api_key: str):
        self.client = httpx.AsyncClient(
            base_url=base_url.rstrip("/"),
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
        embedding: list[float] | None = None,
        source_agent_id: str | None = None,
    ):
        payload = MemoryStore(
            path=path,
            content=content,
            metadata=metadata or {},
            tags=tags,
            embedding_model=embedding_model,
            embedding=embedding,
            source_agent_id=source_agent_id,
        )
        resp = await self.client.post("/v1/memories", json=payload.model_dump(exclude_none=True))
        resp.raise_for_status()
        return resp.json()

    async def retrieve(self, path_prefix: str, query: str = "", limit: int = 10):
        payload = MemoryRetrieve(path_prefix=path_prefix, query=query, limit=limit)
        resp = await self.client.post("/v1/retrieve", json=payload.model_dump(exclude_none=True))
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

    async def subscribe(self, _handler):
        """Reserved for a future /v1/events stream."""
        raise NotImplementedError("PCMI subscribe() is not implemented yet")
