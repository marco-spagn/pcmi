"""Tests for FIX-9: session methods in Python SDK.
Run with: pytest sdk/python/pcmi/test_sessions.py
"""
from unittest.mock import AsyncMock, MagicMock, patch
import pytest
from .client import PCMIClient


@pytest.fixture
def mock_client():
    client = PCMIClient(base_url="http://localhost:8000", api_key="test-key")
    client.client = MagicMock()
    return client


@pytest.mark.asyncio
async def test_create_session_posts_to_sessions(mock_client):
    mock_resp = MagicMock()
    mock_resp.raise_for_status = MagicMock()
    mock_resp.json.return_value = {"id": "sess-123", "status": "active"}
    mock_client.client.post = AsyncMock(return_value=mock_resp)

    result = await mock_client.create_session(agent_id="agent-1",
                                               metadata={"k": "v"})
    mock_client.client.post.assert_called_once_with(
        "/v1/sessions", json={"agent_id": "agent-1", "metadata": {"k": "v"}}
    )
    assert result["id"] == "sess-123"


@pytest.mark.asyncio
async def test_end_session_deletes_session(mock_client):
    mock_resp = MagicMock()
    mock_resp.raise_for_status = MagicMock()
    mock_resp.json.return_value = {"id": "sess-123", "status": "ended"}
    mock_client.client.delete = AsyncMock(return_value=mock_resp)

    result = await mock_client.end_session("sess-123")
    mock_client.client.delete.assert_called_once_with("/v1/sessions/sess-123")
    assert result["status"] == "ended"


@pytest.mark.asyncio
async def test_store_session_memory(mock_client):
    mock_resp = MagicMock()
    mock_resp.raise_for_status = MagicMock()
    mock_client.client.post = AsyncMock(return_value=mock_resp)

    await mock_client.store_session_memory(
        "sess-123", "root.test", "content", tags=["a"]
    )
    mock_client.client.post.assert_called_once_with(
        "/v1/sessions/sess-123/memories",
        json={"path": "root.test", "content": "content", "tags": ["a"]},
    )


@pytest.mark.asyncio
async def test_list_session_memories(mock_client):
    mock_resp = MagicMock()
    mock_resp.raise_for_status = MagicMock()
    mock_resp.json.return_value = {"entries": [], "total": 0}
    mock_client.client.get = AsyncMock(return_value=mock_resp)

    await mock_client.list_session_memories("sess-123", limit=10,
                                             path_prefix="root.test")
    mock_client.client.get.assert_called_once_with(
        "/v1/sessions/sess-123/memories",
        params={"limit": 10, "path_prefix": "root.test"},
    )


@pytest.mark.asyncio
async def test_promote_session(mock_client):
    mock_resp = MagicMock()
    mock_resp.raise_for_status = MagicMock()
    mock_resp.json.return_value = {"promoted": 3}
    mock_client.client.post = AsyncMock(return_value=mock_resp)

    result = await mock_client.promote_session("sess-123",
                                                target_prefix="root.agent")
    mock_client.client.post.assert_called_once_with(
        "/v1/sessions/sess-123/promote",
        json={"target_prefix": "root.agent"},
    )
    assert result["promoted"] == 3
