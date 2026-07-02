# Entity extraction profiles (design examples)

Reference JSON for the proposed **domain profile** mechanism described in
[docs/cognitive-graph-entities.md](../../docs/cognitive-graph-entities.md).

These files are **not consumed by PCMI today** — they document the intended
contract for Phase A (LLM slot extraction) and Phase B (entity vertex promotion).

| File | Use case |
|------|----------|
| [`soc.siem.v1.profile.json`](soc.siem.v1.profile.json) | SOC / SIEM incidents (maps to `examples/soc-incident-graph/`) |
| [`generic.record.v1.profile.json`](generic.record.v1.profile.json) | Domain-agnostic minimum slots for non-SOC tenants |

When implemented, tenants will register profiles via admin API; the LLM worker
fills **all** `required_slots` keys (nullable values allowed).
