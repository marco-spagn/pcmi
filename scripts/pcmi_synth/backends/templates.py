"""Deterministic template-based generators for non-SOC presets."""

from __future__ import annotations

import random
import uuid
from datetime import datetime, timedelta, timezone
from typing import Any

from ..models import GenerateOptions, MemoryRecord
from ..presets import Preset

_AGENT_NS = uuid.UUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")


def _agent_uuid(name: str) -> str:
    return str(uuid.uuid5(_AGENT_NS, name))


def _rand_ts(rng: random.Random, days_back: int = 180) -> datetime:
    now = datetime(2026, 5, 16, 12, 0, 0, tzinfo=timezone.utc)
    return now - timedelta(seconds=rng.randint(0, days_back * 24 * 3600))


def _exact_assignments(rng: random.Random, categories: tuple[str, ...], weights: tuple[float, ...], total: int) -> list[str]:
    counts: dict[str, int] = {}
    assigned = 0
    for name, pct in zip(categories, weights, strict=True):
        n = int(round(pct * total))
        counts[name] = n
        assigned += n
    drift = total - assigned
    if drift and categories:
        counts[categories[0]] = counts.get(categories[0], 0) + drift
    expanded: list[str] = []
    for name, n in counts.items():
        expanded.extend([name] * n)
    rng.shuffle(expanded)
    return expanded


def _build_path(preset: Preset, category: str, record_id: str, shard_id: int | None) -> str:
    cat = category.replace("-", "_")
    rid = record_id.replace("-", "_").lower()
    if shard_id is not None:
        return f"{preset.path_prefix}.shard_{shard_id:03d}.{cat}.{rid}"
    return f"{preset.path_prefix}.{cat}.{rid}"


def _finance_content(rng: random.Random, category: str, record_id: str, ts: datetime) -> str:
    amount = rng.randint(500, 250_000)
    currency = rng.choice(["USD", "EUR", "GBP"])
    account = f"ACC-{rng.randint(10000, 99999)}"
    templates = {
        "payment_anomaly": (
            f"Payment anomaly {record_id} — {ts.isoformat()}\n"
            f"Outbound transfer {amount:,} {currency} to counterparty {account} "
            f"deviates {rng.randint(40, 400)}% from 30-day baseline. Rule PAY-{rng.randint(100,999)}."
        ),
        "fraud_alert": (
            f"Fraud alert {record_id} — {ts.isoformat()}\n"
            f"Card-not-present velocity: {rng.randint(3, 40)} tx in {rng.randint(5, 60)} min. "
            f"Total exposure {amount:,} {currency}. Score {rng.uniform(0.7, 0.99):.2f}."
        ),
        "aml_review": (
            f"AML review {record_id} — {ts.isoformat()}\n"
            f"Structuring pattern across {rng.randint(2, 8)} accounts; aggregate {amount:,} {currency}. "
            f"Jurisdiction flags: {rng.choice(['EU', 'US', 'APAC'])}."
        ),
        "chargeback": (
            f"Chargeback {record_id} — {ts.isoformat()}\n"
            f"Dispute reason {rng.choice(['fraud', 'service', 'duplicate'])}; amount {amount:,} {currency}; "
            f"merchant MID-{rng.randint(1000, 9999)}."
        ),
        "reconciliation_gap": (
            f"Reconciliation gap {record_id} — {ts.isoformat()}\n"
            f"Ledger vs processor mismatch {amount:,} {currency} on settlement date {ts.date()}. "
            f"Unmatched rows: {rng.randint(1, 120)}."
        ),
        "regulatory_flag": (
            f"Regulatory flag {record_id} — {ts.isoformat()}\n"
            f"Reportable event under {rng.choice(['SOX', 'PCI', 'GDPR workflow'])}; "
            f"exposure {amount:,} {currency}; ticket REG-{rng.randint(1000, 9999)}."
        ),
    }
    return templates.get(category, templates["payment_anomaly"])


def _advertising_content(rng: random.Random, category: str, record_id: str, ts: datetime) -> str:
    campaign = f"CMP-{rng.randint(1000, 9999)}"
    channel = rng.choice(["search", "social", "display", "video", "email"])
    spend = rng.randint(200, 80_000)
    templates = {
        "ctr_drop": (
            f"CTR drop {record_id} — {ts.isoformat()}\n"
            f"Campaign {campaign} ({channel}): CTR fell {rng.randint(15, 70)}% vs 7d baseline. "
            f"Spend {spend} USD still pacing."
        ),
        "budget_pacing": (
            f"Budget pacing {record_id} — {ts.isoformat()}\n"
            f"Campaign {campaign}: {rng.randint(110, 185)}% daily spend rate; "
            f"projected overrun {rng.randint(5, 40)}% by month end."
        ),
        "creative_fatigue": (
            f"Creative fatigue {record_id} — {ts.isoformat()}\n"
            f"Ad set {campaign}/creative-{rng.randint(1, 20)}: frequency {rng.uniform(4, 12):.1f}, "
            f"CPA up {rng.randint(20, 90)}%."
        ),
        "audience_overlap": (
            f"Audience overlap {record_id} — {ts.isoformat()}\n"
            f"Segments A/B overlap {rng.randint(25, 75)}% on {channel}; "
            f"inflated impressions est. {rng.randint(10, 55)}%."
        ),
        "bid_spike": (
            f"Bid spike {record_id} — {ts.isoformat()}\n"
            f"Campaign {campaign}: CPC +{rng.randint(30, 200)}% hour-over-hour; "
            f"competitor auction pressure suspected."
        ),
        "conversion_anomaly": (
            f"Conversion anomaly {record_id} — {ts.isoformat()}\n"
            f"Campaign {campaign}: conversion rate {rng.uniform(0.1, 3.0):.2f}% "
            f"vs expected {rng.uniform(2, 8):.2f}%; spend {spend} USD."
        ),
    }
    return templates.get(category, templates["ctr_drop"])


def _healthcare_content(rng: random.Random, category: str, record_id: str, ts: datetime) -> str:
    facility = f"FAC-{rng.randint(10, 99)}"
    templates = {
        "scheduling_conflict": (
            f"Scheduling conflict {record_id} — {ts.isoformat()}\n"
            f"Facility {facility}: {rng.randint(2, 12)} double-booked slots in OR block; "
            f"synthetic patient SYN-{rng.randint(10000, 99999)}."
        ),
        "inventory_alert": (
            f"Inventory alert {record_id} — {ts.isoformat()}\n"
            f"SKU MED-{rng.randint(1000, 9999)} below par level at {facility}; "
            f"days of supply {rng.uniform(0.5, 3):.1f}."
        ),
        "claims_denial": (
            f"Claims denial {record_id} — {ts.isoformat()}\n"
            f"Payer code {rng.choice(['CO-97', 'CO-16', 'PR-1'])}; "
            f"amount {rng.randint(100, 15000)} USD; appeal window {rng.randint(15, 45)} days."
        ),
        "quality_metric": (
            f"Quality metric {record_id} — {ts.isoformat()}\n"
            f"Readmission risk bucket elevated for unit {facility}; "
            f"index {rng.uniform(1.1, 2.4):.2f} vs target 1.0."
        ),
        "device_maintenance": (
            f"Device maintenance {record_id} — {ts.isoformat()}\n"
            f"Device DEV-{rng.randint(100, 999)} calibration due; last service "
            f"{rng.randint(90, 400)} days ago at {facility}."
        ),
        "staffing_gap": (
            f"Staffing gap {record_id} — {ts.isoformat()}\n"
            f"Shift {ts.strftime('%Y-%m-%d %H:%M')}: {rng.randint(1, 6)} roles understaffed "
            f"at {facility}; acuity score {rng.randint(3, 5)}/5."
        ),
    }
    return templates.get(category, templates["scheduling_conflict"])


_CONTENT_FN = {
    "finance": _finance_content,
    "advertising": _advertising_content,
    "healthcare": _healthcare_content,
}


def generate_template_records(opts: GenerateOptions, preset: Preset) -> list[MemoryRecord]:
    if preset.name in ("soc", "custom"):
        raise ValueError(f"preset {preset.name!r} must use another backend")

    content_fn = _CONTENT_FN.get(preset.name)
    if content_fn is None:
        raise ValueError(f"no template backend for preset {preset.name!r}")

    rng = random.Random(opts.seed)
    categories = preset.categories
    weights = preset.category_weights
    assignments = _exact_assignments(rng, categories, weights, opts.num_records)

    shard_size = max(1, opts.shard_size) if opts.use_sharding else 0
    records: list[MemoryRecord] = []

    for i, category in enumerate(assignments):
        record_id = f"{preset.name.upper()}-{opts.seed:04d}-{i+1:05d}"
        ts = _rand_ts(rng)
        shard_id = (i // shard_size) if shard_size else None
        path = _build_path(preset, category, record_id, shard_id)
        content = content_fn(rng, category, record_id, ts)
        agent_name = f"{preset.agent_namespace}-{rng.randint(1, 12)}"
        campaign_id = None
        if rng.random() < opts.campaign_ratio:
            campaign_id = f"campaign-{rng.randint(1, max(3, opts.num_records // 50))}"

        metadata: dict[str, Any] = {
            "record_id": record_id,
            "category": category,
            "preset": preset.name,
            "reported_at": ts.isoformat(),
            "source_agent": agent_name,
            "test_data_seed": str(opts.seed),
            "test_data_version": opts.test_data_version,
            "shard_id": shard_id,
        }
        if campaign_id:
            metadata["campaign_id"] = campaign_id

        tags = [
            f"preset:{preset.name}",
            f"category:{category}",
            "synthetic",
            f"seed:{opts.seed}",
            f"v{opts.test_data_version}",
        ]
        if campaign_id:
            tags.append(f"campaign:{campaign_id.lower()}")
        if shard_id is not None:
            tags.append(f"shard:{shard_id:03d}")

        records.append(
            MemoryRecord(
                path=path,
                content=content,
                metadata=metadata,
                tags=tags,
                source_agent_id=_agent_uuid(agent_name),
                valid_from=ts.isoformat(),
            )
        )
    return records
