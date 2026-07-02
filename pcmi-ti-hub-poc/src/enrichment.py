"""Persist PCMI-discovered correlations back into the Cognitive Graph.

Closing the loop: the correlations PCMI surfaces in Phase 3 (via LLM-embedding
retrieval) are written back as typed `related` edges. Two outcomes make the point:

* **validated** — the discovered link already exists in the graph (the analyst had
  hand-curated it in the source data). PCMI *independently rediscovered* it.
* **added** — the discovered link is new; PCMI grows the memory graph with a
  cross-vendor association the source data did not contain.

Every persisted edge is tagged `discovered_by = pcmi-semantic-retrieval` so it is
distinguishable from analyst-authored links.
"""
from __future__ import annotations

from typing import Any

from .pcmi_client import PCMIClient, PCMIError


class Enricher:
    def __init__(self, client: PCMIClient):
        self.client = client

    async def _edge_exists(self, a: int, b: int) -> bool:
        """True if a graph edge already connects a and b in either direction."""
        for src, dst in ((a, b), (b, a)):
            rel = await self.client.graph_related(src, depth=1)
            if dst in {e["id"] for e in (rel.get("entries") or [])}:
                return True
        return False

    async def persist_correlations(self, correlations: list[dict[str, Any]]) -> dict[str, list]:
        """`correlations`: list of {a_id, b_id, label, score, shared_aliases, kind}.

        Writes a `related` edge for each that is not already present; classifies
        each as validated (pre-existing) or added (new).
        """
        added: list[dict] = []
        validated: list[dict] = []
        for c in correlations:
            a, b = c["a_id"], c["b_id"]
            if a == b:
                continue
            if await self._edge_exists(a, b):
                validated.append(c)
                continue
            metadata: dict[str, Any] = {
                "discovered_by": "pcmi-semantic-retrieval",
                "correlation_kind": c["kind"],
                "score": c["score"],
            }
            if c.get("shared_aliases"):
                metadata["shared_aliases"] = c["shared_aliases"]
            try:
                await self.client.link(a, b, "related", metadata=metadata)
                added.append(c)
            except PCMIError:
                # A concurrent/duplicate edge — treat as already present.
                validated.append(c)
        return {"added": added, "validated": validated}
