from pydantic import BaseModel, Field
from typing import Any


class MemoryStore(BaseModel):
    path: str
    content: str
    metadata: dict[str, Any] = Field(default_factory=dict)
    tags: list[str] | None = None
    embedding_model: str | None = None
    embedding_space: str | None = None
    embedding: list[float] | None = None
    source_agent_id: str | None = None
    encrypt_content: bool | None = None
    expires_at: str | None = None


class MemoryRetrieve(BaseModel):
    path_prefix: str = ""
    query: str = ""
    limit: int = 10
    as_of: str | None = None
    source_agent_id: str | None = None
    embedding_space: str | None = None
    tags: list[str] | None = None
    tags_match: str | None = None


class CompactMemory(BaseModel):
    path: str
    keep_superseded: int = 20


class IngestEvent(BaseModel):
    event_type: str
    agent_id: str | None = None
    correlation_id: str | None = None
    payload: dict[str, Any] = Field(default_factory=dict)


class MemoryRollback(BaseModel):
    path: str
    version: int | None = None
    as_of: str | None = None
