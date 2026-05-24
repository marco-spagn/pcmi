"""Register PCMI tools for AutoGen AgentChat (demo without starting an LLM)."""

from __future__ import annotations

import asyncio
import os

from pcmi_tools import build_pcmi_tools, pcmi_retrieve, pcmi_store


async def demo() -> None:
    if not os.environ.get("PCMI_API_KEY"):
        raise SystemExit("Set PCMI_API_KEY (and optionally PCMI_BASE_URL)")

    tools = build_pcmi_tools()
    path = "root.autogen.demo"
    await pcmi_store(path, "hello from AutoGen tools")
    hits = await pcmi_retrieve("root.autogen", "hello", 3)
    print("retrieve:", hits)
    print("registered:", [t.name for t in tools])


if __name__ == "__main__":
    asyncio.run(demo())
