"""Pull **real STIX 2.1 bundles from TI Mindmap HUB's live platform** via its MCP
server (`TI_HUB_MODE=live`).

TI Mindmap HUB hosts an MCP server (Streamable HTTP transport) that exposes 25
tools, including ``list_stix_bundles`` and ``get_stix_bundle``. This module speaks
just enough MCP (JSON-RPC 2.0) to: initialise a session, list available bundles,
and fetch each STIX 2.1 bundle — which then flow through ``stix_ingest.parse_bundle``
exactly like any other STIX input.

Requires an API key (``tim_...``) from https://ti-mindmap-hub.com (account
settings), passed via ``TIHUB_API_KEY`` and sent as the ``X-API-Key`` header.

Config (env):
  TIHUB_API_KEY   tim_xxxxxxxx           (required)
  TIHUB_MCP_URL   defaults to the hosted endpoint below
  TIHUB_MCP_LIMIT max bundles to pull    (default 20)
"""
from __future__ import annotations

import json
from typing import Any

import httpx

DEFAULT_MCP_URL = "https://ti-mindmap-mcp.happyfield-b3b5145b.westeurope.azurecontainerapps.io/mcp"


class MCPError(RuntimeError):
    pass


def _parse_rpc_response(resp: httpx.Response) -> dict[str, Any]:
    """A Streamable-HTTP MCP POST returns either application/json or an SSE stream
    (text/event-stream). Return the JSON-RPC message either way."""
    ctype = resp.headers.get("content-type", "")
    if "text/event-stream" in ctype:
        message: dict[str, Any] = {}
        for line in resp.text.splitlines():
            if line.startswith("data:"):
                chunk = line[5:].strip()
                if chunk and chunk != "[DONE]":
                    try:
                        message = json.loads(chunk)
                    except json.JSONDecodeError:
                        continue
        return message
    return resp.json() if resp.text else {}


def _tool_text(result: dict[str, Any]) -> str:
    """Extract the text payload from an MCP tools/call result."""
    content = (result.get("result") or {}).get("content") or []
    parts = [c.get("text", "") for c in content if isinstance(c, dict) and c.get("type") == "text"]
    return "\n".join(p for p in parts if p)


def extract_bundle_ids(list_text: str) -> list[str]:
    """From a ``list_stix_bundles`` text result, pull candidate article/report ids.
    Defensive: the exact shape may vary, so accept several id-ish keys."""
    try:
        data = json.loads(list_text)
    except json.JSONDecodeError:
        return []
    rows = data.get("bundles") or data.get("items") or data.get("results") or data if isinstance(data, list) else []
    if isinstance(data, dict) and not rows:
        rows = next((v for v in data.values() if isinstance(v, list)), [])
    ids: list[str] = []
    for row in rows or []:
        if isinstance(row, dict):
            for key in ("article_id", "report_id", "id", "articleId", "reportId"):
                if row.get(key):
                    ids.append(str(row[key]))
                    break
        elif isinstance(row, str):
            ids.append(row)
    return ids


def bundle_from_text(text: str) -> dict[str, Any] | None:
    """A ``get_stix_bundle`` text result is a STIX bundle JSON string."""
    try:
        obj = json.loads(text)
    except json.JSONDecodeError:
        return None
    if isinstance(obj, dict) and obj.get("type") == "bundle":
        return obj
    # Some servers wrap it: {"bundle": {...}} or {"stix": {...}}
    for key in ("bundle", "stix", "data"):
        inner = obj.get(key) if isinstance(obj, dict) else None
        if isinstance(inner, dict) and inner.get("type") == "bundle":
            return inner
    return None


async def pull_stix_bundles(
    api_key: str, *, mcp_url: str = DEFAULT_MCP_URL, limit: int = 20, timeout: float = 45.0
) -> list[dict[str, Any]]:
    """Handshake with the MCP server, list bundles, and fetch each as STIX 2.1."""
    if not api_key:
        raise MCPError("TIHUB_API_KEY is required for TI_HUB_MODE=live (get a tim_... key at ti-mindmap-hub.com)")

    headers = {
        "X-API-Key": api_key,
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
    }
    async with httpx.AsyncClient(timeout=timeout) as client:
        # 1) initialize
        init = {
            "jsonrpc": "2.0", "id": 1, "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "pcmi-ti-hub-poc", "version": "1.0"},
            },
        }
        r = await client.post(mcp_url, headers=headers, json=init)
        if r.status_code == 401:
            raise MCPError("MCP authentication failed (401) — check TIHUB_API_KEY")
        r.raise_for_status()
        session_id = r.headers.get("mcp-session-id") or r.headers.get("Mcp-Session-Id")
        if session_id:
            headers["Mcp-Session-Id"] = session_id
        # 2) notifications/initialized
        await client.post(mcp_url, headers=headers,
                          json={"jsonrpc": "2.0", "method": "notifications/initialized"})

        async def call_tool(name: str, arguments: dict, call_id: int) -> dict:
            payload = {"jsonrpc": "2.0", "id": call_id, "method": "tools/call",
                       "params": {"name": name, "arguments": arguments}}
            resp = await client.post(mcp_url, headers=headers, json=payload)
            resp.raise_for_status()
            return _parse_rpc_response(resp)

        # 3) list bundles
        listing = await call_tool("list_stix_bundles", {"limit": limit, "offset": 0}, 2)
        ids = extract_bundle_ids(_tool_text(listing))

        bundles: list[dict[str, Any]] = []
        # Some servers may already return full bundles in the listing text.
        direct = bundle_from_text(_tool_text(listing))
        if direct:
            bundles.append(direct)

        for i, article_id in enumerate(ids[:limit], start=3):
            got = await call_tool("get_stix_bundle", {"article_id": article_id}, i)
            bundle = bundle_from_text(_tool_text(got))
            if bundle:
                bundles.append(bundle)

    return bundles
