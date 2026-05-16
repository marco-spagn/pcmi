#!/usr/bin/env python3
"""Manual SDK smoke (HTTP). From sdk/python with venv active:
  export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
  python smoke.py
"""
import asyncio
import os

from pcmi import PCMIClient


async def main() -> None:
    base = os.environ.get("PCMI_BASE_URL", "http://localhost:8000")
    key = os.environ.get("PCMI_API_KEY", "testkey123")
    path = "root.sdk.python.smoke"

    async with PCMIClient(base, key) as c:
        await c.store(
            path,
            "hello from python sdk",
            tags=["sdk-smoke"],
            embedding_model="unspecified",
        )
        out = await c.retrieve(path, tags=["sdk-smoke"], tags_match="all", limit=5)
        print("retrieve total:", out["total"])
        compact = await c.compact(path, keep_superseded=20)
        print("compact:", compact)


if __name__ == "__main__":
    asyncio.run(main())
