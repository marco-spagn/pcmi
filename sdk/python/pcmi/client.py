import httpx
from .models import MemoryStore, MemoryRetrieve

class PCMIClient:
    def __init__(self, base_url: str, api_key: str):
        self.client = httpx.AsyncClient(
            base_url=base_url,
            headers={"X-API-Key": api_key, "Content-Type": "application/json"}
        )

    async def store(self, path: str, content: str, metadata: dict = None):
        payload = MemoryStore(path=path, content=content, metadata=metadata or {})
        resp = await self.client.post("/v1/memories", json=payload.dict())
        return resp.json()

    async def retrieve(self, path_prefix: str, query: str, limit: int = 10):
        payload = MemoryRetrieve(path_prefix=path_prefix, query=query, limit=limit)
        resp = await self.client.post("/v1/retrieve", json=payload.dict())
        return resp.json()