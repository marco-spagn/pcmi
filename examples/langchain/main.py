"""Run LangChain tools against a live PCMI API (no LLM required)."""

from __future__ import annotations

import os

from pcmi_tools import PCMI_TOOLS, pcmi_retrieve, pcmi_store


def demo() -> None:
    if not os.environ.get("PCMI_API_KEY"):
        raise SystemExit("Set PCMI_API_KEY (and optionally PCMI_BASE_URL)")

    path = "root.langchain.demo"
    pcmi_store.invoke({"path": path, "content": "hello from LangChain tools"})
    hits = pcmi_retrieve.invoke({"path_prefix": "root.langchain", "query": "hello", "limit": 3})
    print("retrieve:", hits)
    print("registered tools:", [t.name for t in PCMI_TOOLS])


if __name__ == "__main__":
    demo()
