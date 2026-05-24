"""LangChain tools backed by PCMI HTTP (store / retrieve / session working memory)."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from langchain_core.tools import tool

from pcmi_http import create_session, end_session, retrieve, session_list, session_store, store


@tool
def pcmi_store(path: str, content: str) -> str:
    """Persist a memory at path in PCMI long-term storage."""
    result = store(path, content, metadata={"framework": "langchain"})
    return str(result.get("id", result))


@tool
def pcmi_retrieve(path_prefix: str, query: str = "", limit: int = 5) -> str:
    """Semantic retrieve from PCMI for a path prefix."""
    result = retrieve(path_prefix, query, limit)
    entries = result.get("entries") or result.get("results") or []
    return str(entries[:limit])


@tool
def pcmi_session_remember(note_path: str, content: str) -> str:
    """Create a session, store working memory, list it, then end the session."""
    session = create_session(agent_id="langchain-example")
    sid = session["id"]
    try:
        session_store(sid, note_path, content)
        listed = session_list(sid)
        return str(listed.get("entries") or listed.get("memories") or listed)
    finally:
        end_session(sid)


PCMI_TOOLS = [pcmi_store, pcmi_retrieve, pcmi_session_remember]
