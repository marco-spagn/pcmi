"""Ingest synthetic records into PCMI (SDK + optional JSONL backup)."""

from __future__ import annotations

import asyncio
import json
import logging
import os
from pathlib import Path
from typing import Any

from .models import MemoryRecord

LOG = logging.getLogger("pcmi_synth.ingest")

PCMIClient: Any = None


def _lazy_sdk() -> None:
    global PCMIClient
    if PCMIClient is not None:
        return
    try:
        from pcmi import PCMIClient as _C  # type: ignore[no-redef]
    except ImportError:
        from pcmi_sdk import PCMIClient as _C  # type: ignore[import-not-found]
    PCMIClient = _C


def write_jsonl(records: list[MemoryRecord], path: str, tenant_id: str) -> int:
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    with p.open("w", encoding="utf-8") as fh:
        for rec in records:
            fh.write(json.dumps(rec.to_jsonl_row(tenant_id), ensure_ascii=False) + "\n")
    LOG.info("Wrote JSONL backup: %s (%d records)", path, len(records))
    return len(records)


async def ingest_records(
    records: list[MemoryRecord],
    *,
    api_url: str,
    api_key: str,
    batch_size: int = 50,
    throttle_ms: int = 0,
) -> tuple[int, int]:
    _lazy_sdk()
    ok = fail = 0
    async with PCMIClient(base_url=api_url, api_key=api_key) as client:
        store_batch = getattr(client, "store_batch", client.batch_store)
        total = len(records)
        for start in range(0, total, batch_size):
            chunk = records[start : start + batch_size]
            payload = [r.to_batch_item() for r in chunk]
            try:
                resp = await store_batch(payload)
            except Exception as exc:
                fail += len(chunk)
                LOG.error("Batch %d-%d failed: %s", start + 1, start + len(chunk), exc)
                continue
            results = (resp or {}).get("results", []) if isinstance(resp, dict) else []
            stored = sum(1 for item in results if item.get("status") in ("stored", "skipped"))
            ok += stored
            fail += len(chunk) - stored
            if throttle_ms > 0 and (start + batch_size) < total:
                await asyncio.sleep(throttle_ms / 1000.0)
    return ok, fail


async def publish_refine_shard(
    *,
    redis_url: str,
    redis_channel: str,
    tenant_id: str,
    path_prefix: str,
    reason: str,
) -> bool:
    wrapper = {
        "Type": "memory.refine.requested",
        "Payload": {
            "tenant_id": tenant_id,
            "path_prefix": path_prefix,
            "reason": reason,
        },
    }
    try:
        import redis.asyncio as aioredis
    except ImportError:
        LOG.warning("redis package not installed; skip publish")
        return False
    try:
        r = aioredis.from_url(redis_url, encoding="utf-8", decode_responses=True)
        n = await r.publish(redis_channel, json.dumps(wrapper))
        await r.aclose()
        LOG.info("Published refine to %s (subscribers=%d)", redis_channel, n)
        return True
    except Exception as exc:
        LOG.error("Redis publish failed: %s", exc)
        return False


def ingest_sync(**kwargs: Any) -> tuple[int, int]:
    return asyncio.run(ingest_records(**kwargs))
