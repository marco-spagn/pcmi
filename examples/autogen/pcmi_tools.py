"""AutoGen AgentChat FunctionTools backed by PCMI HTTP."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from autogen_core.tools import FunctionTool

from pcmi_http import retrieve, store


async def pcmi_store(path: str, content: str) -> str:
    result = store(path, content, metadata={"framework": "autogen"})
    return str(result.get("id", result))


async def pcmi_retrieve(path_prefix: str, query: str = "", limit: int = 5) -> str:
    result = retrieve(path_prefix, query, limit)
    entries = result.get("entries") or result.get("results") or []
    return str(entries[:limit])


def build_pcmi_tools() -> list[FunctionTool]:
    return [
        FunctionTool(pcmi_store, description="Store a memory path/content in PCMI"),
        FunctionTool(pcmi_retrieve, description="Retrieve memories from PCMI by path prefix"),
    ]
