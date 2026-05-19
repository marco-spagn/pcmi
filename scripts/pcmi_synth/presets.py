"""Built-in use-case presets for synthetic data generation."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Preset:
    name: str
    description: str
    path_prefix: str
    tenant_slug: str
    agent_namespace: str
    categories: tuple[str, ...]
  # weights same length as categories
    category_weights: tuple[float, ...]
    llm_system_hint: str


PRESETS: dict[str, Preset] = {
    "soc": Preset(
        name="soc",
        description="Security operations center incidents (MITRE-aligned, campaign clustering)",
        path_prefix="root.security.incidents.soc",
        tenant_slug="soc-test",
        agent_namespace="soc-analyst",
        categories=(),  # SOC uses dedicated generator
        category_weights=(),
        llm_system_hint=(
            "Enterprise SOC security incidents: severity, MITRE tactic/technique, "
            "true/false positive mix, affected systems, detection narrative."
        ),
    ),
    "finance": Preset(
        name="finance",
        description="Financial operations alerts (fraud, payments, compliance)",
        path_prefix="root.finance.events",
        tenant_slug="finance-test",
        agent_namespace="finance-ops",
        categories=(
            "payment_anomaly",
            "fraud_alert",
            "aml_review",
            "chargeback",
            "reconciliation_gap",
            "regulatory_flag",
        ),
        category_weights=(0.22, 0.20, 0.15, 0.13, 0.18, 0.12),
        llm_system_hint=(
            "Financial operations alerts for a global bank: payments, fraud, AML, "
            "chargebacks, reconciliation, regulatory flags. Realistic amounts and IDs."
        ),
    ),
    "advertising": Preset(
        name="advertising",
        description="Digital advertising campaign telemetry and anomalies",
        path_prefix="root.marketing.ads",
        tenant_slug="ads-test",
        agent_namespace="ads-optimizer",
        categories=(
            "ctr_drop",
            "budget_pacing",
            "creative_fatigue",
            "audience_overlap",
            "bid_spike",
            "conversion_anomaly",
        ),
        category_weights=(0.20, 0.18, 0.16, 0.14, 0.16, 0.16),
        llm_system_hint=(
            "Digital advertising campaign events: CTR drops, budget pacing, creative fatigue, "
            "audience overlap, bid spikes, conversion anomalies. Include channel and campaign IDs."
        ),
    ),
    "healthcare": Preset(
        name="healthcare",
        description="Clinical and operations signals (synthetic, non-PHI)",
        path_prefix="root.healthcare.ops",
        tenant_slug="health-test",
        agent_namespace="care-coordinator",
        categories=(
            "scheduling_conflict",
            "inventory_alert",
            "claims_denial",
            "quality_metric",
            "device_maintenance",
            "staffing_gap",
        ),
        category_weights=(0.18, 0.16, 0.20, 0.15, 0.16, 0.15),
        llm_system_hint=(
            "Healthcare operations events without real PHI: scheduling, inventory, claims, "
            "quality metrics, device maintenance, staffing. Use synthetic patient IDs only."
        ),
    ),
    "custom": Preset(
        name="custom",
        description="User-defined domain (requires --domain; LLM recommended)",
        path_prefix="root.custom.synthetic",
        tenant_slug="custom-test",
        agent_namespace="custom-agent",
        categories=("event",),
        category_weights=(1.0,),
        llm_system_hint="User-defined domain for synthetic operational memories.",
    ),
}


def get_preset(name: str) -> Preset:
    key = name.strip().lower()
    if key not in PRESETS:
        known = ", ".join(sorted(PRESETS))
        raise ValueError(f"Unknown preset {name!r}. Choose one of: {known}")
    return PRESETS[key]


def list_presets() -> list[Preset]:
    return [PRESETS[k] for k in sorted(PRESETS)]
