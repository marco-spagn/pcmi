#!/usr/bin/env python3
"""CLI: python3 -m pcmi_synth (from repo root: PYTHONPATH=scripts python3 -m pcmi_synth)."""

from __future__ import annotations

import argparse
import asyncio
import logging
import os
import sys

from .generate import generate_records
from .ingest import ingest_records, write_jsonl
from .models import GenerateOptions
from .presets import list_presets


def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="pcmi_synth",
        description="Generate synthetic PCMI memories for distillation E2E tests.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    sub = p.add_subparsers(dest="command", required=True)

    lp = sub.add_parser("list", help="List built-in presets")
    lp.set_defaults(command="list")

    g = sub.add_parser("generate", help="Generate (and optionally ingest) synthetic data")
    g.add_argument(
        "--preset",
        "-p",
        required=True,
        help="Use case: soc, finance, advertising, healthcare, custom",
    )
    g.add_argument("--num", "-n", type=int, default=1000, help="Number of records")
    g.add_argument("--seed", "-s", type=int, default=42, help="RNG seed (deterministic templates)")
    g.add_argument("--path-prefix", default="", help="Override ltree path prefix")
    g.add_argument("--shard-size", type=int, default=10, help="Records per distillation shard")
    g.add_argument("--no-sharding", action="store_true", help="Disable .shard_NNN path segments")
    g.add_argument("--output", "-o", default="./synthetic_backup.jsonl", help="JSONL backup path")
    g.add_argument("--tenant-id", default=os.getenv("TENANT_ID", ""))
    g.add_argument("--api-url", default=os.getenv("PCMI_BASE_URL", os.getenv("PCMI_API_URL", "http://localhost:8000")))
    g.add_argument("--api-key", default=os.getenv("PCMI_API_KEY", ""))
    g.add_argument("--batch-size", type=int, default=50)
    g.add_argument("--throttle-ms", type=int, default=0)
    g.add_argument("--dry-run", action="store_true", help="JSONL only, no API calls")
    g.add_argument("--skip-ingest", action="store_true", help="Generate + JSONL only")
    g.add_argument("--llm", action="store_true", help="Use OpenAI to author record content")
    g.add_argument("--domain", default="", help="Custom domain text (required for preset=custom with --llm)")
    g.add_argument("--llm-model", default=os.getenv("DISTILLATION_MODEL", ""))
    g.add_argument("--llm-batch-size", type=int, default=20)
    g.set_defaults(command="generate")
    return p


def main(argv: list[str] | None = None) -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s | %(levelname)-8s | %(name)s | %(message)s",
    )
    args = _build_parser().parse_args(argv)

    if args.command == "list":
        for preset in list_presets():
            print(f"{preset.name:12}  {preset.description}")
            print(f"             path: {preset.path_prefix}")
        return 0

    if not args.api_key and not args.dry_run and not args.skip_ingest:
        print("error: --api-key or PCMI_API_KEY required for ingest (or use --dry-run)", file=sys.stderr)
        return 2

    opts = GenerateOptions(
        preset=args.preset,
        num_records=args.num,
        seed=args.seed,
        path_prefix=args.path_prefix,
        shard_size=args.shard_size,
        use_sharding=not args.no_sharding,
        tenant_id=args.tenant_id,
        use_llm=args.llm,
        domain=args.domain,
        llm_model=args.llm_model,
        llm_batch_size=args.llm_batch_size,
    )
    result = generate_records(opts)
    tenant = args.tenant_id or "00000000-0000-0000-0000-000000000001"
    write_jsonl(result.records, args.output, tenant)

    print(
        f"Generated {len(result.records)} records | preset={result.preset} | "
        f"seed={result.seed} | shards≈{result.num_shards} | jsonl={args.output}"
    )

    if args.dry_run or args.skip_ingest:
        return 0

    ok, fail = asyncio.run(
        ingest_records(
            result.records,
            api_url=args.api_url,
            api_key=args.api_key,
            batch_size=args.batch_size,
            throttle_ms=args.throttle_ms,
        )
    )
    print(f"Ingest complete: ok={ok} fail={fail}")
    return 1 if fail else 0


if __name__ == "__main__":
    raise SystemExit(main())
