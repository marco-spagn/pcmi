#!/usr/bin/env python3
"""Validate the realistic Cognitive Graph JSON dataset."""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from pathlib import Path


LINK_TYPES = {"causal", "temporal", "supports", "contradicts", "related"}
KINDS = {"campaign", "alert", "evidence", "hypothesis", "postmortem"}
LTREE = re.compile(r"^root(\.[a-z0-9_]+)+$")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("dataset", nargs="?", default="graph_realistic_large.json")
    args = parser.parse_args()

    path = Path(args.dataset)
    data = json.loads(path.read_text(encoding="utf-8"))
    nodes = data.get("nodes", [])
    links = data.get("links", [])
    errors: list[str] = []

    keys: set[str] = set()
    paths: set[str] = set()
    kind_counts: Counter[str] = Counter()
    disposition_counts: Counter[str] = Counter()
    tactic_counts: Counter[str] = Counter()

    for idx, node in enumerate(nodes):
        key = node.get("key", "")
        node_path = node.get("path", "")
        metadata = node.get("metadata", {})
        kind = metadata.get("kind", "")
        if not key:
            errors.append(f"node {idx} missing key")
        if key in keys:
            errors.append(f"duplicate node key {key}")
        keys.add(key)
        if not LTREE.match(node_path):
            errors.append(f"bad ltree path for {key}: {node_path}")
        if node_path in paths:
            errors.append(f"duplicate path {node_path}")
        paths.add(node_path)
        if kind not in KINDS:
            errors.append(f"node {key} has bad kind {kind!r}")
        kind_counts[kind] += 1
        if metadata.get("disposition"):
            disposition_counts[metadata["disposition"]] += 1
        if metadata.get("mitre_tactic"):
            tactic_counts[metadata["mitre_tactic"]] += 1
        if len(node.get("content", "")) < 40:
            errors.append(f"node {key} content too short")

    edge_seen: set[tuple[str, str, str]] = set()
    link_type_counts: Counter[str] = Counter()
    for idx, link in enumerate(links):
        src = link.get("from", "")
        dst = link.get("to", "")
        typ = link.get("type", "")
        edge = (src, dst, typ)
        if src not in keys:
            errors.append(f"link {idx} dangling source {src}")
        if dst not in keys:
            errors.append(f"link {idx} dangling target {dst}")
        if typ not in LINK_TYPES:
            errors.append(f"link {idx} bad type {typ}")
        if src == dst:
            errors.append(f"link {idx} self-loop {src}")
        if edge in edge_seen:
            errors.append(f"duplicate edge {edge}")
        edge_seen.add(edge)
        link_type_counts[typ] += 1
        weight = link.get("weight")
        if not isinstance(weight, (float, int)) or weight <= 0 or weight > 1:
            errors.append(f"link {idx} bad weight {weight}")

    connected = {src for src, _, _ in edge_seen} | {dst for _, dst, _ in edge_seen}
    missing_link_types = LINK_TYPES - set(link_type_counts)
    if missing_link_types:
        errors.append(f"missing link types: {sorted(missing_link_types)}")
    if len(nodes) < 1000:
        errors.append(f"dataset too small: {len(nodes)} nodes")
    if len(links) < len(nodes):
        errors.append(f"graph too sparse: {len(links)} links for {len(nodes)} nodes")
    if len(tactic_counts) < 10:
        errors.append(f"too few MITRE tactics covered: {len(tactic_counts)}")
    for required_kind in KINDS:
        if kind_counts[required_kind] == 0:
            errors.append(f"missing node kind {required_kind}")
    for required_disposition in {"true_positive", "false_positive", "benign_true_positive", "duplicate"}:
        if disposition_counts[required_disposition] == 0:
            errors.append(f"missing disposition {required_disposition}")

    print("=" * 72)
    print("REALISTIC COGNITIVE GRAPH VALIDATION")
    print("=" * 72)
    print(f"dataset: {path}")
    print(f"nodes: {len(nodes)}")
    print(f"links: {len(links)}")
    print(f"connected nodes: {len(connected)} ({len(connected) * 100 // max(1, len(nodes))}%)")
    print(f"isolated nodes: {len(nodes) - len(connected)}")
    print(f"kinds: {dict(sorted(kind_counts.items()))}")
    print(f"dispositions: {dict(sorted(disposition_counts.items()))}")
    print(f"link types: {dict(sorted(link_type_counts.items()))}")
    print(f"MITRE tactics covered: {len(tactic_counts)}")

    if errors:
        print()
        print(f"FAILED with {len(errors)} error(s):")
        for error in errors[:30]:
            print(f"  - {error}")
        sys.exit(1)

    print()
    print("OK: realistic graph dataset is structurally coherent")


if __name__ == "__main__":
    main()
