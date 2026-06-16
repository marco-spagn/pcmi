#!/usr/bin/env python3
"""Structural smoke tests for load_to_pcmi.py.

These tests avoid live PCMI calls and focus on the loader invariants that keep
graph edges aligned with the dataset currently on disk.
"""

import csv
import json
import tempfile
from pathlib import Path

import load_to_pcmi as loader


def write_csv(path, rows):
    with open(path, "w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=rows[0].keys())
        writer.writeheader()
        writer.writerows(rows)


def with_temp_loader_files(fn):
    old = (loader.NODES_CSV, loader.LINKS_CSV, loader.ID_MAP, loader.ID_MAP_META)
    with tempfile.TemporaryDirectory() as td:
        root = Path(td)
        loader.NODES_CSV = str(root / "nodes.csv")
        loader.LINKS_CSV = str(root / "links.csv")
        loader.ID_MAP = str(root / "id_map.json")
        loader.ID_MAP_META = str(root / "id_map.meta.json")
        try:
            fn(root)
        finally:
            loader.NODES_CSV, loader.LINKS_CSV, loader.ID_MAP, loader.ID_MAP_META = old


def test_stale_id_map_is_ignored():
    def run(root):
        write_csv(Path(loader.NODES_CSV), [
            {"external_id": "a", "path": "root.a", "content": "A", "tags": "graph"},
        ])
        write_csv(Path(loader.LINKS_CSV), [
            {"from_external_id": "a", "to_external_id": "a", "link_type": "related", "rationale": "self"},
        ])
        loader.save_id_map({"a": 1})

        write_csv(Path(loader.NODES_CSV), [
            {"external_id": "b", "path": "root.b", "content": "B", "tags": "graph"},
        ])
        got = loader.load_id_map()
        assert got == {}, f"stale id_map should be ignored after CSV change, got {got}"

    with_temp_loader_files(run)


def test_collect_edges_uses_loaded_node_map_not_first_link_rows():
    def run(root):
        write_csv(Path(loader.NODES_CSV), [
            {"external_id": "n1", "path": "root.n1", "content": "N1", "tags": "graph"},
            {"external_id": "n3", "path": "root.n3", "content": "N3", "tags": "graph"},
        ])
        write_csv(Path(loader.LINKS_CSV), [
            {"from_external_id": "n2", "to_external_id": "n4", "link_type": "related", "rationale": "not loaded"},
            {"from_external_id": "n1", "to_external_id": "n3", "link_type": "supports", "rationale": "loaded late"},
        ])
        edges, skipped = loader.collect_edges({"n1": 101, "n3": 103})
        assert skipped == 1, f"expected one skipped dangling edge, got {skipped}"
        assert edges == [("memory.101", "memory.103", "supports", "loaded late")], edges

    with_temp_loader_files(run)


if __name__ == "__main__":
    test_stale_id_map_is_ignored()
    test_collect_edges_uses_loaded_node_map_not_first_link_rows()
    print("OK: load_to_pcmi structural tests passed")
