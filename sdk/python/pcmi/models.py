from pydantic import BaseModel, Field
from typing import Any


class MemoryStore(BaseModel):
    path: str
    content: str
    metadata: dict[str, Any] = Field(default_factory=dict)
    tags: list[str] | None = None
    embedding_model: str | None = None
    embedding: list[float] | None = None
    source_agent_id: str | None = None


class MemoryRetrieve(BaseModel):
    path_prefix: str = ""
    query: str = ""
    limit: int = 10
