#!/usr/bin/env python3
"""Register SOC + generic extraction profiles on a live PCMI API."""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent
BASE = os.environ.get("PCMI_BASE_URL", "http://localhost:8000").rstrip("/")
KEY = os.environ.get("PCMI_API_KEY", "testkey123")

PROFILES = (
    ("soc.siem.v1", "root.soc", ROOT / "soc.siem.v1.profile.json"),
    ("generic.record.v1", "root.realistic_graph", ROOT / "generic.record.v1.profile.json"),
)


def put(profile_id: str, path_prefix: str, profile_path: Path) -> None:
    profile = json.loads(profile_path.read_text(encoding="utf-8"))
    payload = json.dumps(
        {"path_prefix": path_prefix, "enabled": True, "profile": profile}
    ).encode()
    req = urllib.request.Request(
        f"{BASE}/v1/extraction-profiles/{profile_id}",
        data=payload,
        method="PUT",
        headers={"Content-Type": "application/json", "X-API-Key": KEY},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        if resp.status not in (200, 201):
            raise RuntimeError(f"{profile_id}: HTTP {resp.status}")


def main() -> int:
    failed = 0
    for profile_id, prefix, path in PROFILES:
        try:
            put(profile_id, prefix, path)
            print(f"  ✓ {profile_id} (path_prefix={prefix})")
        except (urllib.error.HTTPError, OSError, RuntimeError) as e:
            print(f"  ✗ {profile_id}: {e}", file=sys.stderr)
            failed += 1
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
