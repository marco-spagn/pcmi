"""Optional OpenAI-backed synthetic record generation."""

from __future__ import annotations

import json
import logging
import os
import random
import uuid
from datetime import datetime, timezone
from typing import Any

from ..models import GenerateOptions, MemoryRecord
from ..presets import Preset

LOG = logging.getLogger("pcmi_synth.llm")

_AGENT_NS = uuid.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")


def _agent_uuid(name: str) -> str:
    return str(uuid.uuid5(_AGENT_NS, name))


def _domain_hint(opts: GenerateOptions, preset: Preset) -> str:
    if opts.domain.strip():
        return opts.domain.strip()
    if preset.name == "custom":
        raise ValueError("--domain is required when preset=custom and --llm is set")
    return preset.llm_system_hint


def generate_llm_records(opts: GenerateOptions, preset: Preset) -> list[MemoryRecord]:
    api_key = os.getenv("OPENAI_API_KEY", "").strip()
    if not api_key:
        raise RuntimeError("OPENAI_API_KEY is required for --llm generation")

    try:
        from openai import OpenAI
    except ImportError as exc:
        raise RuntimeError("Install openai: pip install openai") from exc

    model = opts.llm_model or os.getenv("DISTILLATION_MODEL", "gpt-4o-mini")
    client = OpenAI(api_key=api_key)
    rng = random.Random(opts.seed)
    domain = _domain_hint(opts, preset)
    categories = preset.categories or ("event",)
    weights = preset.category_weights or (1.0,)
    assignments = []
    for _ in range(opts.num_records):
        assignments.append(rng.choices(list(categories), weights=list(weights), k=1)[0])

    shard_size = max(1, opts.shard_size) if opts.use_sharding else 0
    records: list[MemoryRecord] = []
    batch_size = max(1, min(opts.llm_batch_size, 50))

    for start in range(0, opts.num_records, batch_size):
        chunk_cats = assignments[start : start + batch_size]
        n = len(chunk_cats)
        prompt = {
            "domain": domain,
            "preset": preset.name,
            "seed": opts.seed,
            "path_prefix": opts.path_prefix,
            "count": n,
            "categories": chunk_cats,
            "instructions": (
                "Return a JSON array of objects. Each object must have: "
                "path (ltree-safe, dots only), content (multi-line string, 80-400 words), "
                "metadata (object with record_id, category, reported_at ISO8601), "
                "tags (string array). Paths must start with the given path_prefix. "
                "No real PII; use synthetic IDs."
            ),
        }
        response = client.chat.completions.create(
            model=model,
            temperature=0.7,
            response_format={"type": "json_object"},
            messages=[
                {
                    "role": "system",
                    "content": (
                        "You generate synthetic operational memory records for testing "
                        "a knowledge distillation pipeline. Output valid JSON only."
                    ),
                },
                {
                    "role": "user",
                    "content": json.dumps(prompt),
                },
            ],
        )
        raw = response.choices[0].message.content or "{}"
        parsed = json.loads(raw)
        items = parsed.get("records") or parsed.get("items") or parsed
        if isinstance(items, dict):
            items = [items]
        if not isinstance(items, list):
            raise ValueError(f"LLM returned unexpected shape: {type(items)}")

        for j, item in enumerate(items):
            if j >= n:
                break
            global_idx = start + j
            category = chunk_cats[j]
            record_id = item.get("metadata", {}).get("record_id") or (
                f"{preset.name.upper()}-{opts.seed:04d}-{global_idx+1:05d}"
            )
            shard_id = (global_idx // shard_size) if shard_size else None
            path = item.get("path") or ""
            if not path.startswith(opts.path_prefix):
                seg = category.replace("-", "_")
                rid = str(record_id).replace("-", "_").lower()
                if shard_id is not None:
                    path = f"{opts.path_prefix}.shard_{shard_id:03d}.{seg}.{rid}"
                else:
                    path = f"{opts.path_prefix}.{seg}.{rid}"

            ts = datetime.now(tz=timezone.utc).isoformat()
            agent_name = f"{preset.agent_namespace}-llm-{rng.randint(1, 8)}"
            metadata = dict(item.get("metadata") or {})
            metadata.setdefault("record_id", record_id)
            metadata.setdefault("category", category)
            metadata.setdefault("preset", preset.name)
            metadata.setdefault("reported_at", ts)
            metadata.setdefault("test_data_seed", str(opts.seed))
            metadata.setdefault("test_data_version", opts.test_data_version)
            metadata.setdefault("source_agent", agent_name)
            metadata.setdefault("shard_id", shard_id)

            tags = list(item.get("tags") or [])
            for t in (f"preset:{preset.name}", f"category:{category}", "synthetic", "llm"):
                if t not in tags:
                    tags.append(t)

            records.append(
                MemoryRecord(
                    path=path,
                    content=str(item.get("content") or f"Synthetic {category} record {record_id}"),
                    metadata=metadata,
                    tags=tags,
                    source_agent_id=_agent_uuid(agent_name),
                    valid_from=str(item.get("valid_from") or ts),
                )
            )

        LOG.info("LLM batch %d-%d generated (%d records total)", start + 1, start + n, len(records))

    if len(records) < opts.num_records:
        LOG.warning("LLM produced %d/%d records; padding with deterministic stubs", len(records), opts.num_records)
        from .templates import generate_template_records

        pad_opts = GenerateOptions(
            preset=preset.name if preset.name != "custom" else "finance",
            num_records=opts.num_records - len(records),
            seed=opts.seed + 999,
            path_prefix=opts.path_prefix,
            shard_size=opts.shard_size,
            use_sharding=opts.use_sharding,
            tenant_id=opts.tenant_id,
            test_data_version=opts.test_data_version,
        )
        if preset.name in ("finance", "advertising", "healthcare"):
            records.extend(generate_template_records(pad_opts, preset))

    return records[: opts.num_records]
