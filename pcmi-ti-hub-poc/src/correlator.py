"""Cross-vendor correlation via PCMI retrieval.

The same actor is named differently by each vendor (CrowdStrike "COZY BEAR",
Microsoft "Midnight Blizzard", both = APT29). Lexical/keyword search cannot bridge
those surface strings. PCMI's **hybrid retrieval with LLM embeddings**
(text-embedding-3-small) can: two findings that describe the same tradecraft land
next to each other in vector space regardless of code-name.

Two passes:

  * **Semantic actor pass** — use each actor/campaign memory's own description as a
    semantic query and see which *other-vendor* actor memories come back. Each
    candidate pair is then cross-checked against the structured alias metadata
    (``actor`` / ``public_alias``): a shared canonical identity (e.g. ``apt29``,
    ``cozy bear``) upgrades a semantic *candidate* to a *confirmed* correlation.

  * **TTP pass** — shared-technique probes ("supply chain", "OAuth", "zero-day",
    "AI malware", ...); hits spanning >= 2 vendors are cross-vendor correlations.
"""
from __future__ import annotations

import re
from typing import Any

from .pcmi_client import PCMIClient

# Namespace all CTI memories live under (matches the dataset paths).
NAMESPACE = "root.cti.vendor_reports"

# (label, query) — TTP themes from the brief. Compact terms so the lexical
# component matches; embeddings broaden the recall.
TTP_PROBES: list[tuple[str, str]] = [
    ("AI-enabled malware", "AI malware"),
    ("Supply-chain compromise", "supply chain"),
    ("OAuth / token abuse", "OAuth"),
    ("Zero-day exploitation", "zero-day"),
    ("Ransomware operations", "ransomware"),
    ("DLL side-loading", "DLL side-loading"),
    ("Phishing / social engineering", "phishing"),
    ("DPRK financial operations", "DPRK"),
    ("Edge-device intrusion", "edge device"),
]

# Nation / agency words are attributions, not actor identities — they must not
# confirm "same actor" (two different DPRK groups are still different actors).
_NATION_AGENCY = {
    "gru", "svr", "dprk", "iran", "iran-nexus", "russia", "russian",
    "russia-affiliated", "china", "chinese", "north korea", "pakistan",
    "pakistan-based", "military", "state", "affiliated", "nexus",
}


def vendor_of(entry: dict[str, Any]) -> str | None:
    """Vendor that authored a finding, or None for PCMI-derived memories
    (LLM consolidation/distillation write ``…consolidated`` memories that carry
    no vendor and must not be counted as a source)."""
    return (entry.get("metadata") or {}).get("vendor")


def is_vendor_finding(entry: dict[str, Any]) -> bool:
    """True only for original ingested vendor findings (skip derived memories)."""
    md = entry.get("metadata") or {}
    return bool(md.get("vendor")) and bool(md.get("stix_type"))


def normalize_aliases(metadata: dict[str, Any]) -> set[str]:
    """Canonical identity tokens from an actor's ``actor`` / ``public_alias``.

    Returns discriminative code-names and APT numbers (``apt29``, ``cozy bear``,
    ``nobelium``) — deliberately *excluding* nation/agency words so only genuine
    same-actor overlaps intersect.
    """
    parts = [str(metadata[k]) for k in ("actor", "public_alias") if metadata.get(k)]
    raw = " / ".join(parts)
    tokens: set[str] = set()
    for m in re.finditer(r"apt[-\s]?(\d+)", raw, re.IGNORECASE):
        tokens.add(f"apt{m.group(1)}")
    for part in re.split(r"[/,()]", raw):
        p = re.sub(r"\s+", " ", part.strip().lower())
        p = re.sub(r"apt[-\s]?\d+", "", p).strip()          # apt-number already captured
        p = re.sub(r"\s+(group|subgroup)$", "", p).strip()  # drop generic suffix
        if len(p) >= 3 and p not in _NATION_AGENCY:
            tokens.add(p)
    return tokens


def _lexical_hit(entry: dict[str, Any], tokens: list[str]) -> bool:
    """True when every token appears as a whole word in the entry's text."""
    md = entry.get("metadata") or {}
    blob = " ".join(
        [entry.get("content", ""), " ".join(entry.get("tags") or []), " ".join(str(v) for v in md.values())]
    ).lower()
    return all(re.search(rf"\b{re.escape(tok)}\b", blob) for tok in tokens)


def lexical_contains(entry: dict[str, Any], query: str) -> bool:
    """Whole-word lexical check of a free-text probe against an entry's text.

    Used to keep attribution honest: semantic retrieval recalls neighbours, but a
    "vendor X documents Y" claim is only made when X's memory actually contains Y.
    """
    tokens = [t for t in re.split(r"[^a-z0-9]+", query.lower()) if len(t) >= 2]
    return bool(tokens) and _lexical_hit(entry, tokens)


def _group_by_vendor(entries: list[dict[str, Any]]) -> dict[str, list[dict[str, Any]]]:
    out: dict[str, list[dict[str, Any]]] = {}
    for e in entries:
        v = vendor_of(e)
        if v:  # skip PCMI-derived (vendorless) memories
            out.setdefault(v, []).append(e)
    return out


class Correlator:
    def __init__(self, client: PCMIClient):
        self.client = client

    async def _probe(self, query: str, limit: int = 12) -> tuple[list[dict], dict[str, list[dict]]]:
        entries = await self.client.retrieve_entries(NAMESPACE, query=query, limit=limit)
        return entries, _group_by_vendor(entries)

    async def semantic_actor_correlations(
        self, subjects: list[dict[str, Any]], *, min_score: float = 0.30
    ) -> list[dict[str, Any]]:
        """Semantic same-actor discovery across vendors.

        ``subjects`` = list of {id, vendor, name, content, aliases:set}. For each
        subject, its description is used as a semantic query; other-vendor actor
        memories above ``min_score`` become candidate correlations, confirmed when
        alias sets intersect.
        """
        by_id = {s["id"]: s for s in subjects}
        subject_ids = set(by_id)
        seen: set[tuple[int, int]] = set()
        results: list[dict[str, Any]] = []

        for s in subjects:
            entries = await self.client.retrieve_entries(NAMESPACE, query=s["content"][:500], limit=8)
            for e in entries:
                if e["id"] not in subject_ids or e["id"] == s["id"]:
                    continue
                if vendor_of(e) == s["vendor"]:
                    continue
                score = float(e.get("relevance_score", 0.0) or 0.0)
                if score < min_score:
                    continue
                pair = (min(s["id"], e["id"]), max(s["id"], e["id"]))
                if pair in seen:
                    continue
                seen.add(pair)
                other = by_id[e["id"]]
                shared = sorted(s["aliases"] & other["aliases"])
                results.append(
                    {
                        "a": {"id": s["id"], "name": s["name"], "vendor": s["vendor"]},
                        "b": {"id": other["id"], "name": other["name"], "vendor": other["vendor"]},
                        "score": round(score, 3),
                        "confirmed": bool(shared),
                        "shared_aliases": shared,
                    }
                )

        results.sort(key=lambda r: (not r["confirmed"], -r["score"]))
        return results

    async def ttp_correlations(self) -> list[dict[str, Any]]:
        """TTP probes documented (lexically) by >= 2 distinct vendors.

        Semantic retrieval always fills ``limit`` with the nearest neighbours, so
        a raw top-K vendor count would flag even TTPs absent from the corpus. We
        therefore use the semantic query for *recall* but only attribute a TTP to
        a vendor when that vendor's memory text actually contains the term
        (lexical *precision*) — an honest "who documented this".
        """
        results: list[dict[str, Any]] = []
        for label, query in TTP_PROBES:
            # Wide limit so lexical confirmation sees every candidate, not just the
            # top-K semantic neighbours (deterministic "who documented this TTP").
            entries = await self.client.retrieve_entries(NAMESPACE, query=query, limit=50)
            tokens = [t for t in re.split(r"[^a-z0-9]+", query.lower()) if len(t) >= 2]
            confirmed = [e for e in entries if is_vendor_finding(e) and _lexical_hit(e, tokens)]
            by_vendor = _group_by_vendor(confirmed)
            if len(by_vendor) >= 2:
                results.append(
                    {
                        "label": label,
                        "query": query,
                        "vendors": sorted(by_vendor),
                        "n_vendors": len(by_vendor),
                        "n_memories": len(confirmed),
                    }
                )
        return results
