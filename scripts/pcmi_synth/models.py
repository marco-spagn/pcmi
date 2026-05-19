"""Shared types for synthetic PCMI memory generation."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass(frozen=True)
class GenerateOptions:
    preset: str
    num_records: int
    seed: int
    path_prefix: str
    shard_size: int = 10
    use_sharding: bool = True
    tenant_id: str = ""
    test_data_version: str = "1.0"
    use_llm: bool = False
    domain: str = ""
    llm_model: str = ""
    llm_batch_size: int = 20
    campaign_ratio: float = 0.65


@dataclass
class MemoryRecord:
    path: str
    content: str
    metadata: dict[str, Any]
    tags: list[str]
    source_agent_id: str
    valid_from: str
    version: int = 1

    def to_batch_item(self) -> dict[str, Any]:
        return {
            "path": self.path,
            "content": self.content,
            "metadata": self.metadata,
            "tags": self.tags,
            "source_agent_id": self.source_agent_id,
        }

    def to_jsonl_row(self, tenant_id: str) -> dict[str, Any]:
        return {
            "tenant_id": tenant_id,
            "path": self.path,
            "content": self.content,
            "metadata": self.metadata,
            "tags": self.tags,
            "version": self.version,
            "valid_from": self.valid_from,
            "source_agent_id": self.source_agent_id,
        }


@dataclass
class GenerateResult:
    records: list[MemoryRecord] = field(default_factory=list)
    preset: str = ""
    seed: int = 0
    num_shards: int = 0
