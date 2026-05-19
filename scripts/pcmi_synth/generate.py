"""Orchestrate synthetic record generation."""

from __future__ import annotations

import logging

from .backends.llm import generate_llm_records
from .backends.soc import generate_soc_records
from .backends.templates import generate_template_records
from .models import GenerateOptions, GenerateResult, MemoryRecord
from .presets import Preset, get_preset

LOG = logging.getLogger("pcmi_synth")


def generate_records(opts: GenerateOptions) -> GenerateResult:
    preset = get_preset(opts.preset)
    path_prefix = opts.path_prefix or preset.path_prefix

    run_opts = GenerateOptions(
        preset=preset.name,
        num_records=opts.num_records,
        seed=opts.seed,
        path_prefix=path_prefix,
        shard_size=opts.shard_size,
        use_sharding=opts.use_sharding,
        tenant_id=opts.tenant_id,
        test_data_version=opts.test_data_version,
        use_llm=opts.use_llm,
        domain=opts.domain,
        llm_model=opts.llm_model,
        llm_batch_size=opts.llm_batch_size,
        campaign_ratio=opts.campaign_ratio,
    )

    LOG.info(
        "Generating preset=%s num=%d seed=%d sharding=%s shard_size=%d llm=%s",
        preset.name,
        run_opts.num_records,
        run_opts.seed,
        run_opts.use_sharding,
        run_opts.shard_size,
        run_opts.use_llm,
    )

    records: list[MemoryRecord]
    if run_opts.use_llm:
        records = generate_llm_records(run_opts, preset)
    elif preset.name == "soc":
        records = generate_soc_records(run_opts)
    elif preset.name == "custom":
        raise ValueError("preset=custom requires --llm and --domain (deterministic custom not supported)")
    else:
        records = generate_template_records(run_opts, preset)

    shard_size = max(1, run_opts.shard_size) if run_opts.use_sharding else 0
    num_shards = (
        (len(records) + shard_size - 1) // shard_size if shard_size else 1
    )

    return GenerateResult(
        records=records,
        preset=preset.name,
        seed=run_opts.seed,
        num_shards=num_shards,
    )
