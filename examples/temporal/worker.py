"""Run Temporal worker for PCMI demo workflows."""

from __future__ import annotations

import asyncio
import os

from temporalio.client import Client
from temporalio.worker import Worker

from activities import retrieve_memory, store_memory
from workflows import PcmiDemoWorkflow

TASK_QUEUE = "pcmi-demo"


async def main() -> None:
    addr = os.environ.get("TEMPORAL_ADDRESS", "localhost:7233")
    client = await Client.connect(addr, namespace=os.environ.get("TEMPORAL_NAMESPACE", "default"))
    worker = Worker(
        client,
        task_queue=TASK_QUEUE,
        workflows=[PcmiDemoWorkflow],
        activities=[store_memory, retrieve_memory],
    )
    await worker.run()


if __name__ == "__main__":
    asyncio.run(main())
