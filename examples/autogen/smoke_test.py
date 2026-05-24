"""Structural checks for examples/autogen."""

from __future__ import annotations

import ast
import os
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))


class AutoGenExampleStructural(unittest.TestCase):
    def test_readme_exists(self) -> None:
        readme = Path(__file__).with_name("README.md")
        self.assertTrue(readme.is_file())
        self.assertIn("autogen", readme.read_text(encoding="utf-8").lower())

    def test_sources_parse(self) -> None:
        for name in ("main.py", "pcmi_tools.py"):
            ast.parse(Path(__file__).with_name(name).read_text(encoding="utf-8"))

    def test_requirements(self) -> None:
        req = Path(__file__).with_name("requirements.txt").read_text(encoding="utf-8")
        self.assertIn("autogen-agentchat", req)


class AutoGenExampleLive(unittest.TestCase):
    @unittest.skipUnless(os.environ.get("PCMI_SMOKE_LIVE") == "1", "set PCMI_SMOKE_LIVE=1")
    def test_async_roundtrip(self) -> None:
        import asyncio

        from pcmi_tools import pcmi_retrieve, pcmi_store

        async def run() -> None:
            await pcmi_store("root.autogen.smoke", "autogen smoke")
            out = await pcmi_retrieve("root.autogen.smoke", "smoke", 2)
            self.assertIn("smoke", out.lower())

        asyncio.run(run())


if __name__ == "__main__":
    unittest.main()
