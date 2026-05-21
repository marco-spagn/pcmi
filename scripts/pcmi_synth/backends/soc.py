"""SOC preset: wraps the legacy enterprise incident generator."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

from ..models import GenerateOptions, MemoryRecord

_SCRIPTS_DIR = Path(__file__).resolve().parents[2]


def _load_soc_module():
    path = _SCRIPTS_DIR / "generate_soc_incidents_enterprise_v2.py"
    name = "generate_soc_incidents_enterprise_v2"
    if name in sys.modules:
        return sys.modules[name]
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise ImportError(f"cannot load SOC generator from {path}")
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


def generate_soc_records(opts: GenerateOptions) -> list[MemoryRecord]:
    soc = _load_soc_module()
    cfg = soc.GeneratorConfig(
        num_incidents=opts.num_records,
        tenant_id=opts.tenant_id or "00000000-0000-0000-0000-000000000001",
        api_url="http://localhost:8000",
        api_key="unused",
        seed=opts.seed,
        output="",
        refine_path_prefix=opts.path_prefix,
        test_data_version=opts.test_data_version,
        use_sharding=opts.use_sharding,
        shard_size=opts.shard_size,
    )
    incidents = soc.generate_incidents(cfg)
    out: list[MemoryRecord] = []
    for inc in incidents:
        out.append(
            MemoryRecord(
                path=inc.path,
                content=inc.content,
                metadata=inc.metadata,
                tags=inc.tags,
                source_agent_id=inc.source_agent_id,
                valid_from=inc.valid_from,
                version=inc.version,
            )
        )
    return out
