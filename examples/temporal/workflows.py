"""Temporal workflows (deterministic): orchestrate PCMI activities."""

from __future__ import annotations

from datetime import timedelta

from temporalio import workflow

with workflow.unsafe.imports_passed_through():
    from activities import retrieve_memory, store_memory


@workflow.defn
class PcmiDemoWorkflow:
    """Store a memory, then retrieve under the same path prefix."""

    @workflow.run
    async def run(self, path: str, content: str) -> dict:
        stored = await workflow.execute_activity(
            store_memory,
            args=[path, content],
            start_to_close_timeout=timedelta(minutes=2),
        )
        prefix = path.rsplit(".", 1)[0] if "." in path else path
        retrieved = await workflow.execute_activity(
            retrieve_memory,
            args=[prefix, "", 10],
            start_to_close_timeout=timedelta(minutes=2),
        )
        return {"stored": stored, "retrieve": retrieved}
