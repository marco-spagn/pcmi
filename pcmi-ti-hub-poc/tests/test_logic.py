"""Pure-logic unit tests for the PoC (no network / no PCMI required).

Run from the project root:
    python3 -m unittest discover -s tests
"""
import json
import os
import sys
import unittest
from collections import deque

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from src.correlator import (  # noqa: E402
    is_vendor_finding,
    lexical_contains,
    normalize_aliases,
    vendor_of,
)
from src.distillation import (  # noqa: E402
    DISTILLATION_PREFIX,
    DistillationPersister,
    discover_cross_report_knowledge,
    render_discovery_memory,
)
from src.llm_distillation import (  # noqa: E402
    LLM_DISTILLATION_PREFIX,
    LLMDistillationPersister,
    build_entity_context,
    infer_llm_candidates,
    load_fixture_response,
    render_llm_discovery_memory,
    validate_llm_candidates,
)
from src.pcmi_client import PCMIError  # noqa: E402
from src.report import build_markdown_brief  # noqa: E402
from src.stix_ingest import load_stix_bundles, parse_bundle, pcmi_type  # noqa: E402
from src.stix_to_pcmi import map_link_type, sanitize_path, sanitize_segment  # noqa: E402
from src.ti_hub_client import TiHubClient, _parse_date, stix_type  # noqa: E402
from run_poc import effective_promotion_count  # noqa: E402

DATASET = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "examples",
    "vendor_reports_cti_dataset.json",
)


class TestLinkTypeMapping(unittest.TestCase):
    def test_native_types_pass_through(self):
        for t in ("causal", "temporal", "contradicts", "supports", "related"):
            self.assertEqual(map_link_type(t), t)

    def test_stix_verbs_map(self):
        self.assertEqual(map_link_type("uses"), "causal")
        self.assertEqual(map_link_type("targets"), "causal")
        self.assertEqual(map_link_type("exploits"), "causal")
        self.assertEqual(map_link_type("delivers"), "temporal")
        self.assertEqual(map_link_type("drops"), "temporal")
        self.assertEqual(map_link_type("attributed-to"), "supports")
        self.assertEqual(map_link_type("indicates"), "supports")
        self.assertEqual(map_link_type("variant-of"), "related")
        self.assertEqual(map_link_type("related-to"), "related")

    def test_unknown_defaults_to_related(self):
        self.assertEqual(map_link_type("totally-unknown"), "related")
        self.assertEqual(map_link_type(""), "related")

    def test_case_insensitive(self):
        self.assertEqual(map_link_type("USES"), "causal")


class TestLtreeSanitisation(unittest.TestCase):
    def test_segment_lowercases_and_strips(self):
        self.assertEqual(sanitize_segment("Cozy Bear!"), "cozy_bear")
        self.assertEqual(sanitize_segment("APT-29"), "apt_29")
        self.assertEqual(sanitize_segment("  a..b  "), "a_b")

    def test_segment_never_empty(self):
        self.assertEqual(sanitize_segment("!!!"), "x")
        self.assertEqual(sanitize_segment(""), "x")

    def test_path_only_ltree_chars(self):
        p = sanitize_path("root.CTI.vendor reports.Mandiant/Google.m-trends")
        self.assertTrue(all(c.isalnum() or c in "._" for c in p))
        self.assertNotIn("/", p)
        self.assertNotIn(" ", p)

    def test_path_preserves_depth(self):
        self.assertEqual(
            sanitize_path("root.cti.vendor_reports.mandiant.mtrends2026.landscape").count("."),
            5,
        )


class TestAliasNormalisation(unittest.TestCase):
    def test_apt29_detected_from_both_forms(self):
        cozy = normalize_aliases({"actor": "COZY BEAR (APT-29)"})
        midnight = normalize_aliases({"actor": "Midnight Blizzard", "public_alias": "APT-29 / Cozy Bear / NOBELIUM"})
        self.assertIn("apt29", cozy)
        self.assertIn("cozy bear", cozy)
        self.assertIn("apt29", midnight)
        self.assertIn("cozy bear", midnight)
        # the confirmation the PoC relies on:
        self.assertTrue(cozy & midnight)

    def test_nation_agency_excluded(self):
        toks = normalize_aliases({"actor": "Sapphire Sleet", "public_alias": "DPRK"})
        self.assertNotIn("dprk", toks)
        self.assertIn("sapphire sleet", toks)

    def test_different_actors_do_not_intersect(self):
        cozy = normalize_aliases({"actor": "COZY BEAR (APT-29)"})
        fancy = normalize_aliases({"actor": "Forest Blizzard", "public_alias": "APT-28 / Fancy Bear"})
        self.assertFalse(cozy & fancy)  # apt29 != apt28, cozy bear != fancy bear


class TestVendorHelpers(unittest.TestCase):
    def test_vendor_of(self):
        self.assertEqual(vendor_of({"metadata": {"vendor": "CrowdStrike"}}), "CrowdStrike")
        self.assertIsNone(vendor_of({"metadata": {}}))
        self.assertIsNone(vendor_of({}))

    def test_is_vendor_finding(self):
        self.assertTrue(is_vendor_finding({"metadata": {"vendor": "X", "stix_type": "threat-actor"}}))
        self.assertFalse(is_vendor_finding({"metadata": {"vendor": "X"}}))  # derived/consolidated
        self.assertFalse(is_vendor_finding({"metadata": {"stix_type": "note"}}))

    def test_lexical_contains_word_boundary(self):
        e = {"content": "AI-enabled malware and OAuth device-code phishing", "metadata": {}, "tags": []}
        self.assertTrue(lexical_contains(e, "OAuth"))
        self.assertTrue(lexical_contains(e, "AI malware"))
        self.assertFalse(lexical_contains(e, "ransomware"))
        # 'ai' must not match inside 'email'/'chain'
        e2 = {"content": "email chain about supply", "metadata": {}, "tags": []}
        self.assertFalse(lexical_contains(e2, "AI"))


class TestStixTypeMapping(unittest.TestCase):
    def test_kind_mapping(self):
        self.assertEqual(stix_type("threat_actor_activity"), "threat-actor")
        self.assertEqual(stix_type("threat_actor_campaign"), "campaign")
        self.assertEqual(stix_type("malware"), "malware")
        self.assertEqual(stix_type("vulnerability"), "vulnerability")
        self.assertEqual(stix_type("incident"), "incident")
        self.assertEqual(stix_type("statistic"), "note")
        self.assertEqual(stix_type(None), "note")


class TestDateParsing(unittest.TestCase):
    def test_orders_partial_and_full_dates(self):
        self.assertEqual(_parse_date("2026-03-23"), (2026, 3, 23))
        self.assertEqual(_parse_date("2026"), (2026, 1, 1))
        self.assertEqual(_parse_date("2025-2026"), (2025, 1, 1))
        self.assertEqual(_parse_date(None), (9999, 12, 31))
        # Microsoft (2025-05-27) sorts before CrowdStrike (2026-02-24)
        self.assertLess(_parse_date("2025-05-27"), _parse_date("2026-02-24"))


class TestDatasetShaping(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.hub = TiHubClient(DATASET, mode="demo").load()
        cls.reports = cls.hub.reports()

    def test_four_reports_ordered_microsoft_first(self):
        self.assertEqual(len(self.reports), 4)
        self.assertEqual(self.reports[0].vendor, "Microsoft")

    def test_forty_sdos_total(self):
        self.assertEqual(sum(r.node_count for r in self.reports), 40)

    def test_twenty_five_relationships(self):
        self.assertEqual(len(self.hub.relationships()), 25)

    def test_every_sdo_has_valid_stix_type(self):
        valid = {"threat-actor", "campaign", "malware", "vulnerability", "indicator", "incident", "note"}
        for r in self.reports:
            for s in r.sdos:
                self.assertIn(s.stix_type, valid)


class TestBriefRendering(unittest.TestCase):
    def _brief(self):
        return build_markdown_brief(
            dataset="Test CTI",
            generated_at="2026-07-01 12:00 UTC",
            api="http://localhost:8000",
            reports_meta=[{"vendor": "Microsoft", "source": "MSTI", "report_date": "2025-05-27", "node_count": 8}],
            ingest={"total_memories": 40, "links": {"created": 25}, "pre_ingest": [
                {"report_index": 2, "vendor": "Unit 42", "prior_memories": 1}]},
            embeddings={"model": "text-embedding-3-small", "ready": True},
            correlate={
                "confirmed_actor_correlations": [{
                    "a": {"name": "Midnight Blizzard", "vendor": "Microsoft"},
                    "b": {"name": "COZY BEAR", "vendor": "CrowdStrike"},
                    "shared_aliases": ["apt29", "cozy bear"], "score": 0.5}],
                "ttp_correlations": [{"label": "OAuth", "n_vendors": 3,
                                      "vendors": ["A", "B", "C"], "n_memories": 3}],
                "llm_summary": "Executive line.",
            },
            temporal={"demonstrated": True, "path": "root.x", "version_before": 1,
                      "version_now": 2, "as_of_version": 1},
            graph={"start": {"name": "Seashell", "vendor": "Microsoft"}, "subgraph_nodes": 4,
                   "connected_chain": {"hops": 2}, "graph_vertices": 28},
            enrich={"validated": 1, "added": 6},
            session={"subject": "Midnight Blizzard", "working_memories": 4, "promoted": 4,
                     "target_prefix": "root.cti.investigations.midnight_blizzard"},
            distillation={
                "discovered": 3,
                "persisted": 3,
                "links_added": 5,
                "links_validated": 2,
                "candidates": [{
                    "relation_type": "malware_linked_to_actor",
                    "from_name": "PROMPTSTEAL",
                    "to_name": "Forest Blizzard",
                    "confidence": 0.88,
                    "memory_path": "root.cti.distillation.malware_linked_to_actor",
                    "signals": [{"kind": "alias", "value": "apt28"}],
                }],
            },
            llm_distillation={
                "skipped": False,
                "mode": "fixture",
                "model": "gpt-4o-mini",
                "discovered": 2,
                "persisted": 2,
                "rejected": [],
                "links_added": 4,
                "links_validated": 1,
                "candidates": [{
                    "relation_type": "alias_equivalent",
                    "from_name": "Midnight Blizzard",
                    "to_name": "COZY BEAR (APT-29)",
                    "confidence": 0.92,
                    "memory_path": "root.cti.distillation.llm.alias_equivalent",
                    "signals": [{"kind": "alias", "value": "APT-29", "evidence": "APT-29 / Cozy Bear"}],
                }],
            },
            observability={"events_observed": 2, "event_types": ["memory.stored", "memory.updated"],
                           "audit_total": 130},
        )

    def test_brief_has_key_sections(self):
        md = self._brief()
        for token in ("# Cross-Vendor CTI Brief", "Executive summary", "Cross-vendor correlations",
                      "Midnight Blizzard", "COZY BEAR", "apt29", "Temporal evolution",
                      "Cognitive Graph", "Analyst investigation", "OAuth",
                      "Deterministic LLM-style distillation", "LLM inference distillation",
                      "PROMPTSTEAL", "Forest Blizzard",
                      "root.cti.distillation", "root.cti.distillation.llm",
                      "Provenance & observability", "`2`", "`130`"):
            self.assertIn(token, md)
        self.assertNotIn("None", md)  # no unresolved template values leak through

    def test_brief_is_nonempty_markdown(self):
        md = self._brief()
        self.assertGreater(len(md), 500)
        self.assertTrue(md.startswith("# "))


class TestSessionPromotionAccounting(unittest.TestCase):
    def test_fresh_run_uses_newly_promoted_count(self):
        self.assertEqual(effective_promotion_count(promoted_count=4, verified_count=4, working_count=4), 4)

    def test_rerun_uses_verified_existing_dossier(self):
        self.assertEqual(effective_promotion_count(promoted_count=0, verified_count=4, working_count=4), 4)

    def test_missing_dossier_still_fails(self):
        self.assertEqual(effective_promotion_count(promoted_count=0, verified_count=0, working_count=4), 0)


class TestLLMStyleDistillation(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.hub = TiHubClient(DATASET, mode="demo").load()
        cls.reports = cls.hub.reports()

    def test_discovers_requested_examples_without_curated_links(self):
        candidates = discover_cross_report_knowledge(self.reports)
        pairs = {(c["relation_type"], frozenset(c["source_keys"])): c for c in candidates}

        promptsteal = pairs.get(("malware_linked_to_actor", frozenset({"mdt_promptsteal", "ms_forest_blizzard_atc"})))
        self.assertIsNotNone(promptsteal)
        self.assertIn("apt28", {s["value"] for s in promptsteal["signals"]})
        self.assertGreaterEqual(promptsteal["confidence"], 0.85)

        chollima = pairs.get(("campaign_actor_overlap", frozenset({"cs_pressure_chollima_bybit", "ms_sapphire_sleet"})))
        self.assertIsNotNone(chollima)
        self.assertTrue({"DPRK", "cryptocurrency", "macOS"}.issubset({s["value"] for s in chollima["signals"]}))
        self.assertIn("not claimed as the same actor", chollima["explanation"])

        self.assertTrue(all(c["memory_path"].startswith(DISTILLATION_PREFIX) for c in candidates))
        # The discovery function never reads hub.relationships(); removing curated
        # links would not change these runtime candidates.
        self.assertGreaterEqual(len(candidates), 3)

    def test_discovery_memory_contains_evidence(self):
        candidate = next(
            c for c in discover_cross_report_knowledge(self.reports)
            if c["relation_type"] == "campaign_actor_overlap"
        )
        content = render_discovery_memory(candidate)
        for token in ("LLM-style distillation", "Evidence reports", "Signals used", "Confidence", "Explanation"):
            self.assertIn(token, content)


class FakePCMIClient:
    def __init__(self):
        self.ids = {"a": 1, "b": 2}
        self.memories = {}
        self.next_id = 100
        self.links = set()
        self.store_calls = 0
        self.link_calls = 0
        self.related_calls = deque()

    async def get_memory(self, path, **_kwargs):
        if path not in self.memories:
            raise PCMIError(f"GET {path}", 404, "not found")
        return self.memories[path]

    async def store(self, path, content, *, tags=None, metadata=None, importance=None, key=None):
        self.store_calls += 1
        mem_id = self.next_id
        self.next_id += 1
        self.memories[path] = {
            "id": mem_id,
            "path": path,
            "content": content,
            "tags": tags or [],
            "metadata": metadata or {},
            "importance": importance,
            "key": key,
        }
        self.ids[key] = mem_id
        return {"id": mem_id, "status": "stored", "version": 1}

    async def graph_related(self, memory_id, *, depth=3, link_types=None, limit=50, offset=0):
        entries = []
        for a, b in self.links:
            if a == memory_id:
                entries.append({"id": b, "link_type": "related", "depth": 1})
            elif b == memory_id:
                entries.append({"id": a, "link_type": "related", "depth": 1})
        return {"entries": entries}

    async def link(self, from_id, to_id, link_type="related", *, metadata=None):
        self.link_calls += 1
        self.links.add((from_id, to_id))
        return {"id": self.link_calls, "link_type": link_type, "metadata": metadata or {}}


class TestLLMInferenceDistillation(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.hub = TiHubClient(DATASET, mode="demo").load()
        cls.reports = cls.hub.reports()
        cls.ctx = build_entity_context(cls.reports)

    def test_fixture_loads_and_validates_demo_examples(self):
        fixture = load_fixture_response()
        accepted, rejected = validate_llm_candidates(fixture, self.ctx)
        self.assertGreaterEqual(len(accepted), 3, msg=f"rejected={rejected}")
        pairs = {(c["relation_type"], frozenset(c["source_keys"])): c for c in accepted}

        alias = pairs.get(("alias_equivalent", frozenset({"ms_midnight_blizzard_oauth", "cs_cozy_bear_oauth"})))
        self.assertIsNotNone(alias)
        self.assertGreaterEqual(alias["confidence"], 0.85)

        malware = pairs.get(("malware_linked_to_actor", frozenset({"mdt_promptsteal", "ms_forest_blizzard_atc"})))
        self.assertIsNotNone(malware)

        overlap = pairs.get(("campaign_actor_overlap", frozenset({"cs_pressure_chollima_bybit", "ms_sapphire_sleet"})))
        self.assertIsNotNone(overlap)
        self.assertLess(overlap["confidence"], 0.85)

    def test_rejects_hallucinated_evidence(self):
        bad = {
            "candidates": [{
                "relation_type": "alias_equivalent",
                "entity_a": "Midnight Blizzard",
                "entity_b": "COZY BEAR (APT-29)",
                "source_reports": ["Microsoft", "CrowdStrike"],
                "signals": [{"kind": "alias", "value": "APT-29", "evidence": "totally fabricated quote not in corpus"}],
                "confidence": 0.99,
                "explanation": "Fake.",
            }]
        }
        accepted, rejected = validate_llm_candidates(bad, self.ctx)
        self.assertEqual(len(accepted), 0)
        self.assertEqual(len(rejected), 1)
        self.assertIn("evidence", rejected[0]["reason"])

    def test_rejects_same_vendor_pairs(self):
        bad = {
            "candidates": [{
                "relation_type": "alias_equivalent",
                "entity_a": "Midnight Blizzard",
                "entity_b": "Forest Blizzard",
                "source_reports": ["Microsoft"],
                "signals": [{"kind": "alias", "value": "x", "evidence": "Midnight Blizzard sending highly targeted spear-phishing emails"}],
                "confidence": 0.9,
                "explanation": "Same vendor.",
            }]
        }
        accepted, rejected = validate_llm_candidates(bad, self.ctx)
        self.assertEqual(len(accepted), 0)
        self.assertIn("same-vendor", rejected[0]["reason"])

    def test_infer_with_fixture_offline(self):
        async def _run():
            return await infer_llm_candidates(self.ctx, api_key=None)

        import asyncio
        result = asyncio.run(_run())
        self.assertEqual(result["mode"], "fixture")
        self.assertGreaterEqual(result["discovered"], 3)
        self.assertTrue(all(c["memory_path"].startswith(LLM_DISTILLATION_PREFIX) for c in result["accepted"]))

    def test_llm_memory_rendering_labels_inference(self):
        fixture = load_fixture_response()
        accepted, _ = validate_llm_candidates(fixture, self.ctx)
        content = render_llm_discovery_memory(accepted[0])
        self.assertIn("LLM inference distillation", content)
        self.assertIn("Validated against input context", content)


class TestLLMDistillationPersistence(unittest.IsolatedAsyncioTestCase):
    async def test_persist_is_idempotent_on_rerun(self):
        candidate = {
            "candidate_id": "llm.alias_equivalent.a.b",
            "relation_type": "alias_equivalent",
            "pcmi_link_type": "related",
            "confidence": 0.92,
            "from_key": "a",
            "to_key": "b",
            "from_name": "Midnight Blizzard",
            "to_name": "COZY BEAR",
            "source_keys": ["a", "b"],
            "source_paths": ["root.a", "root.b"],
            "source_reports": [{"vendor": "Microsoft", "source": "MSTI"}, {"vendor": "CrowdStrike", "source": "GTR"}],
            "signals": [{"kind": "alias", "value": "apt29", "evidence": "APT-29", "sources": ["a", "b"]}],
            "explanation": "Shared APT29 identity.",
            "memory_path": f"{LLM_DISTILLATION_PREFIX}.alias_equivalent_a_b",
        }
        client = FakePCMIClient()
        persister = LLMDistillationPersister(client)

        first = await persister.persist([candidate])
        second = await persister.persist([candidate])

        self.assertEqual(first["memories_created"], 1)
        self.assertGreaterEqual(first["links_added"], 1)
        self.assertEqual(second["memories_created"], 0)
        self.assertEqual(second["links_added"], 0)
        self.assertGreaterEqual(second["links_validated"], 3)
        self.assertEqual(client.store_calls, 1)


class TestDistillationPersistence(unittest.IsolatedAsyncioTestCase):
    async def test_persist_is_idempotent_on_rerun(self):
        candidate = {
            "candidate_id": "alias_equivalent.a.b",
            "relation_type": "alias_equivalent",
            "pcmi_link_type": "related",
            "confidence": 0.94,
            "from_key": "a",
            "to_key": "b",
            "from_name": "Vendor A Name",
            "to_name": "Vendor B Name",
            "source_keys": ["a", "b"],
            "source_paths": ["root.a", "root.b"],
            "source_reports": [{"vendor": "A", "source": "A report"}, {"vendor": "B", "source": "B report"}],
            "signals": [{"kind": "alias", "value": "apt29", "sources": ["a", "b"]}],
            "explanation": "Shared alias token apt29.",
            "memory_path": f"{DISTILLATION_PREFIX}.alias_equivalent_a_b",
        }
        client = FakePCMIClient()
        persister = DistillationPersister(client)

        first = await persister.persist([candidate])
        second = await persister.persist([candidate])

        self.assertEqual(first["memories_created"], 1)
        self.assertGreaterEqual(first["links_added"], 1)
        self.assertEqual(second["memories_created"], 0)
        self.assertEqual(second["links_added"], 0)
        self.assertGreaterEqual(second["links_validated"], 3)
        self.assertEqual(client.store_calls, 1)


class TestStixIngestion(unittest.TestCase):
    """Live-mode STIX 2.1 ingestion (real HUB format + transcoded bundles)."""

    STIX_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "examples", "tihub_stix")
    HUB_NATIVE = os.path.join(STIX_DIR, "hub_native", "example-apt-campaign.json")

    def test_pcmi_type_mapping(self):
        self.assertEqual(pcmi_type("threat-actor"), "threat-actor")
        self.assertEqual(pcmi_type("intrusion-set"), "threat-actor")
        self.assertEqual(pcmi_type("campaign"), "campaign")
        self.assertEqual(pcmi_type("attack-pattern"), "note")
        self.assertEqual(pcmi_type("vulnerability"), "vulnerability")

    def test_parses_ti_mindmap_hubs_own_published_bundle(self):
        with open(self.HUB_NATIVE, encoding="utf-8") as fh:
            bundle = json.load(fh)
        report, sros = parse_bundle(bundle)
        self.assertTrue(report.sdos)
        self.assertTrue(all(s.metadata.get("stix_type") for s in report.sdos))
        self.assertTrue(all(s.metadata.get("vendor") for s in report.sdos))  # vendor-attributable
        actor = next((s for s in report.sdos if s.stix_type == "threat-actor"), None)
        self.assertIsNotNone(actor)
        self.assertTrue(actor.metadata.get("public_alias") or actor.metadata.get("aliases"))
        self.assertGreaterEqual(len(sros), 1)

    def test_transcoded_bundles_match_demo_shape(self):
        reports, rels, loaded = load_stix_bundles(self.STIX_DIR)
        self.assertEqual(len(reports), 4)  # 4 vendor bundles; hub_native/ is a subdir, not globbed
        self.assertEqual(sum(r.node_count for r in reports), 40)
        self.assertEqual(len(rels), 25)
        keys = {s.key for r in reports for s in r.sdos}
        self.assertTrue(all(r.source in keys and r.target in keys for r in rels))

    def test_live_preserves_apt29_correlation(self):
        reports, _, _ = load_stix_bundles(self.STIX_DIR)
        allsdo = [s for r in reports for s in r.sdos]
        cozy = next(s for s in allsdo if s.metadata.get("actor") == "COZY BEAR (APT-29)")
        mid = next(s for s in allsdo if s.metadata.get("actor") == "Midnight Blizzard")
        self.assertTrue(normalize_aliases(cozy.metadata) & normalize_aliases(mid.metadata))


if __name__ == "__main__":
    unittest.main(verbosity=2)
