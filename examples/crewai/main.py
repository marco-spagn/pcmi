"""Invoke CrewAI @tool wrappers against PCMI (no crew LLM run required)."""

from __future__ import annotations

import os

from pcmi_tools import PCMI_TOOLS, pcmi_retrieve, pcmi_store


def demo() -> None:
    if not os.environ.get("PCMI_API_KEY"):
        raise SystemExit("Set PCMI_API_KEY (and optionally PCMI_BASE_URL)")

    path = "root.crewai.demo"
    pcmi_store.run(path=path, content="hello from CrewAI tools")
    hits = pcmi_retrieve.run(path_prefix="root.crewai", query="hello", limit=3)
    print("retrieve:", hits)
    print("tools:", [t.name for t in PCMI_TOOLS])


if __name__ == "__main__":
    demo()
