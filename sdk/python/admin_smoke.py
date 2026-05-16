#!/usr/bin/env python3
"""Admin SDK smoke (read-only). Requires admin API key (testkey123 in default migrations)."""
import asyncio
import os

from pcmi import PCMIClient


async def main() -> None:
    base = os.environ.get("PCMI_BASE_URL", "http://localhost:8000")
    key = os.environ.get("PCMI_API_KEY", "testkey123")

    async with PCMIClient(base, key) as c:
        tenants = await c.list_tenants(limit=5)
        print("tenants total:", tenants.get("total", 0))
        keys = await c.list_api_keys(limit=5)
        print("api_keys total:", keys.get("total", 0))


if __name__ == "__main__":
    asyncio.run(main())
