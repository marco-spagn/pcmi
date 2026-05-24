"""CrewAI tools backed by PCMI HTTP."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from crewai.tools import tool

from pcmi_http import retrieve, store


@tool("Store memory in PCMI")
def pcmi_store(path: str, content: str) -> str:
    """Persist content at the given hierarchical path."""
    result = store(path, content, metadata={"framework": "crewai"})
    return str(result.get("id", result))


@tool("Retrieve memories from PCMI")
def pcmi_retrieve(path_prefix: str, query: str = "", limit: int = 5) -> str:
    """Semantic search under path_prefix."""
    result = retrieve(path_prefix, query, limit)
    entries = result.get("entries") or result.get("results") or []
    return str(entries[:limit])


PCMI_TOOLS = [pcmi_store, pcmi_retrieve]
