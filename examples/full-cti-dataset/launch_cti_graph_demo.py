#!/usr/bin/env python3
"""Open the CTI threat-actor guided tour on /v1/graph/ui (?demo=cti).

Resolves memory IDs from the live API (no id_map.json required when data is loaded).

Usage:
  python3 launch_cti_graph_demo.py
  python3 launch_cti_graph_demo.py --autostart
  python3 launch_cti_graph_demo.py --no-open
"""
from __future__ import annotations

import argparse
import os
import subprocess
import sys

ROOT = os.path.dirname(os.path.abspath(__file__))
RESOLVER = os.path.join(ROOT, "resolve_demo_ids.py")


def main() -> int:
    parser = argparse.ArgumentParser(description="Launch CTI graph UI guided tour")
    parser.add_argument("--autostart", action="store_true", help="Open with walkthrough=1 (manual steps)")
    parser.add_argument("--no-open", action="store_true", help="Print URL only")
    args = parser.parse_args()

    env = os.environ.copy()
    proc = subprocess.run(
        [sys.executable, RESOLVER, "--url-only"],
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )
    if proc.returncode != 0:
        sys.stderr.write(proc.stderr or proc.stdout)
        return proc.returncode

    url = proc.stdout.strip()
    if args.autostart and "walkthrough=" not in url:
        url += "&walkthrough=1" if "?" in url else "?walkthrough=1"

    print(url)
    if not args.no_open:
        if sys.platform == "darwin":
            subprocess.run(["open", url], check=False)
        else:
            print("Open the URL above in your browser.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
