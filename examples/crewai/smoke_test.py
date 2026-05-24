"""Structural checks for examples/crewai."""

from __future__ import annotations

import ast
import os
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))


class CrewAIExampleStructural(unittest.TestCase):
    def test_readme_exists(self) -> None:
        readme = Path(__file__).with_name("README.md")
        self.assertTrue(readme.is_file())
        self.assertIn("crewai", readme.read_text(encoding="utf-8").lower())

    def test_sources_parse(self) -> None:
        for name in ("main.py", "pcmi_tools.py"):
            ast.parse(Path(__file__).with_name(name).read_text(encoding="utf-8"))

    def test_requirements(self) -> None:
        req = Path(__file__).with_name("requirements.txt").read_text(encoding="utf-8")
        self.assertIn("crewai", req)


class CrewAIExampleLive(unittest.TestCase):
    @unittest.skipUnless(os.environ.get("PCMI_SMOKE_LIVE") == "1", "set PCMI_SMOKE_LIVE=1")
    def test_tool_roundtrip(self) -> None:
        from pcmi_tools import pcmi_retrieve, pcmi_store

        pcmi_store.run(path="root.crewai.smoke", content="crewai smoke")
        out = pcmi_retrieve.run(path_prefix="root.crewai.smoke", query="smoke", limit=2)
        self.assertIn("smoke", out.lower())


if __name__ == "__main__":
    unittest.main()
