"""Start a single PcmiDemoWorkflow execution (requires worker.py running)."""

from __future__ import annotations

import argparse
import asyncio
import os
import uuid

from temporalio.client import Client

from workflows import PcmiDemoWorkflow


async def run(path: str, content: str) -> None:
    addr = os.environ.get("TEMPORAL_ADDRESS", "localhost:7233")
    client = await Client.connect(addr, namespace=os.environ.get("TEMPORAL_NAMESPACE", "default"))
    wid = f"pcmi-demo-{uuid.uuid4().hex[:8]}"
    result = await client.execute_workflow(
        PcmiDemoWorkflow.run,
        args=[path, content],
        id=wid,
        task_queue="pcmi-demo",
    )
    print(result)


if __name__ == "__main__":
    p = argparse.ArgumentParser()
    p.add_argument("path", help="ltree path e.g. root.temporal.demo")
    p.add_argument("content", help="memory body")
    args = p.parse_args()
    asyncio.run(run(args.path, args.content))
