"""Run LlamaIndex FunctionTools against a live PCMI API (no LLM required)."""

from __future__ import annotations

import os

from pcmi_tools import PCMI_TOOLS


def demo() -> None:
    if not os.environ.get("PCMI_API_KEY"):
        raise SystemExit("Set PCMI_API_KEY (and optionally PCMI_BASE_URL)")

    store_tool, retrieve_tool = PCMI_TOOLS
    path = "root.llamaindex.demo"
    store_tool.call(path=path, content="hello from LlamaIndex")
    hits = retrieve_tool.call(path_prefix="root.llamaindex", query="hello", limit=3)
    print("retrieve:", hits)
    print("tools:", [t.metadata.name for t in PCMI_TOOLS])


if __name__ == "__main__":
    demo()
