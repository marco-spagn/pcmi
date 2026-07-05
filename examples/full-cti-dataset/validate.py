#!/usr/bin/env python3
"""Validate combined multi-CTI dataset before load."""
from __future__ import annotations

import os
import sys

ROOT = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, ROOT)

from load_multi_cti import build_combined_dataset  # noqa: E402
from load_to_pcmi import PUBLIC_LINK_TYPES  # noqa: E402


def main() -> int:
    dataset, stats = build_combined_dataset()
    nodes = dataset.get("nodes") or []
    links = dataset.get("links") or []
    keys = {n.get("key") for n in nodes}
    errors = 0
    sources = {s: 0 for s in ("soc", "vendor_reports", "stix", "vendor_intel")}

    for i, n in enumerate(nodes):
        if not n.get("key"):
            print(f"  node[{i}] missing key")
            errors += 1
        if not n.get("path"):
            print(f"  node[{i}] missing path")
            errors += 1
        src = (n.get("metadata") or {}).get("cti_source", "?")
        if src in sources:
            sources[src] += 1

    for i, l in enumerate(links):
        if l.get("from") not in keys or l.get("to") not in keys:
            print(f"  link[{i}] unknown endpoint: {l.get('from')} -> {l.get('to')}")
            errors += 1
        lt = (l.get("type") or "related").lower()
        if lt not in PUBLIC_LINK_TYPES:
            print(f"  link[{i}] invalid type {lt!r}")
            errors += 1

    print(f"  nodes={len(nodes)} links={len(links)} errors={errors}")
    print(
        f"  by source: soc={sources['soc']} vendor_reports={sources['vendor_reports']} "
        f"stix={sources['stix']} vendor_intel={sources['vendor_intel']}"
    )
    print(f"  operational STIX file: {stats.get('operational_stix_json', '?')}")
    print(f"  cross-links from full_cti: {stats.get('cross_links', 0)}")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
