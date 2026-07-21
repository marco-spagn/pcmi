#!/usr/bin/env python3
"""Download public STIX 2.1 bundles from CISA MAR reports and other vendors."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import ssl
import sys
import urllib.error
import urllib.request

try:
    import certifi

    _SSL_CTX = ssl.create_default_context(cafile=certifi.where())
except ImportError:
    _SSL_CTX = ssl.create_default_context()
from datetime import datetime, timezone

ROOT = os.path.dirname(os.path.abspath(__file__))
RAW_DIR = os.path.join(ROOT, "data", "stix_raw")
MANIFEST = os.path.join(RAW_DIR, "manifest.json")

# Public CISA MAR STIX 2.1 bundles (real IOCs, no auth required).
BUNDLES = [
    {
        "id": "MAR-251165",
        "url": "https://www.cisa.gov/sites/default/files/2025-12/MAR-251165.c1.v1.CLEAR_stix2.json",
        "filename": "MAR-251165.c1.v1.CLEAR_stix2.json",
        "campaign": "BRICKSTORM",
        "vendor": "CISA/Mandiant",
        "published": "2025-12-04",
    },
    {
        "id": "MAR-251217",
        "url": "https://www.cisa.gov/sites/default/files/2025-12/MAR-251217.r1.v1.CLEAR_stix2.json",
        "filename": "MAR-251217.r1.v1.CLEAR_stix2.json",
        "campaign": "BRICKSTORM",
        "vendor": "CISA/Mandiant",
        "published": "2025-12-19",
    },
    {
        "id": "MAR-261234",
        "url": "https://www.cisa.gov/sites/default/files/2026-02/MAR-261234.r1.v1.CLEAR_stix2.json",
        "filename": "MAR-261234.r1.v1.CLEAR_stix2.json",
        "campaign": "BRICKSTORM",
        "vendor": "CISA/Mandiant",
        "published": "2026-02-11",
    },
]


def _sha256_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def download_bundle(entry: dict, force: bool = False) -> dict:
    os.makedirs(RAW_DIR, exist_ok=True)
    dest = os.path.join(RAW_DIR, entry["filename"])
    if os.path.isfile(dest) and not force:
        with open(dest, encoding="utf-8") as f:
            obj_count = len(json.load(f).get("objects", []))
        return {
            **entry,
            "path": dest,
            "sha256": _sha256_file(dest),
            "bytes": os.path.getsize(dest),
            "object_count": obj_count,
            "status": "cached",
        }

    req = urllib.request.Request(entry["url"], headers={"User-Agent": "pcmi-cti-downloader/1.0"})
    try:
        with urllib.request.urlopen(req, timeout=120, context=_SSL_CTX) as resp:
            data = resp.read()
    except urllib.error.URLError as exc:
        return {**entry, "path": dest, "status": "error", "error": str(exc)}

    with open(dest, "wb") as f:
        f.write(data)

    # Validate JSON + STIX bundle shape.
    bundle = json.loads(data.decode("utf-8"))
    if bundle.get("type") != "bundle" or not bundle.get("objects"):
        raise ValueError(f"{entry['filename']}: not a valid STIX bundle")

    return {
        **entry,
        "path": dest,
        "sha256": hashlib.sha256(data).hexdigest(),
        "bytes": len(data),
        "object_count": len(bundle["objects"]),
        "status": "downloaded",
    }


def main() -> int:
    ap = argparse.ArgumentParser(description="Download public STIX 2.1 CTI bundles")
    ap.add_argument("--force", action="store_true", help="Re-download even if cached")
    args = ap.parse_args()

    results = []
    errors = 0
    for entry in BUNDLES:
        print(f"  {entry['id']} …", end=" ", flush=True)
        try:
            info = download_bundle(entry, force=args.force)
            results.append(info)
            if info.get("status") == "error":
                print(f"✗ {info.get('error')}")
                errors += 1
            else:
                print(f"✓ {info['status']} ({info.get('bytes', 0)} bytes, {info.get('object_count', '?')} objects)")
        except Exception as exc:
            print(f"✗ {exc}")
            results.append({**entry, "status": "error", "error": str(exc)})
            errors += 1

    manifest = {
        "generated": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "bundles": results,
    }
    with open(MANIFEST, "w", encoding="utf-8") as f:
        json.dump(manifest, f, indent=2)
    print(f"\nmanifest → {MANIFEST}")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
