"""LlamaIndex FunctionTools backed by PCMI HTTP."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from llama_index.core.tools import FunctionTool

from pcmi_http import retrieve, store


def pcmi_store(path: str, content: str) -> str:
    """Store content at path in PCMI."""
    result = store(path, content, metadata={"framework": "llamaindex"})
    return str(result.get("id", result))


def pcmi_retrieve(path_prefix: str, query: str = "", limit: int = 5) -> str:
    """Retrieve memories from PCMI for a path prefix."""
    result = retrieve(path_prefix, query, limit)
    entries = result.get("entries") or result.get("results") or []
    return str(entries[:limit])


PCMI_TOOLS = [
    FunctionTool.from_defaults(fn=pcmi_store, name="pcmi_store"),
    FunctionTool.from_defaults(fn=pcmi_retrieve, name="pcmi_retrieve"),
]
