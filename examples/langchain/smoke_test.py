"""Structural checks for examples/langchain (optional live smoke with PCMI_SMOKE_LIVE=1)."""

from __future__ import annotations

import ast
import os
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))


class LangChainExampleStructural(unittest.TestCase):
    def test_readme_exists(self) -> None:
        readme = Path(__file__).with_name("README.md")
        self.assertTrue(readme.is_file())
        text = readme.read_text(encoding="utf-8")
        self.assertIn("PCMI_API_KEY", text)
        self.assertIn("langchain", text.lower())

    def test_main_parses(self) -> None:
        ast.parse(Path(__file__).with_name("main.py").read_text(encoding="utf-8"))

    def test_tools_module_parses(self) -> None:
        ast.parse(Path(__file__).with_name("pcmi_tools.py").read_text(encoding="utf-8"))

    def test_requirements_pin_langchain(self) -> None:
        req = Path(__file__).with_name("requirements.txt").read_text(encoding="utf-8")
        self.assertIn("langchain-core", req)
        self.assertIn("httpx", req)


class LangChainExampleLive(unittest.TestCase):
    @unittest.skipUnless(os.environ.get("PCMI_SMOKE_LIVE") == "1", "set PCMI_SMOKE_LIVE=1")
    def test_store_and_retrieve(self) -> None:
        from pcmi_tools import pcmi_retrieve, pcmi_store

        path = "root.langchain.smoke"
        pcmi_store.invoke({"path": path, "content": "smoke test payload"})
        out = pcmi_retrieve.invoke({"path_prefix": "root.langchain.smoke", "query": "smoke", "limit": 2})
        self.assertIn("smoke", out.lower())


if __name__ == "__main__":
    unittest.main()
