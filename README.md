# pcmi
# Persistent Cognitive Memory Infrastructure (PCMI) – Go Edition  Memory lives outside agents. Agents are stateless

PCMI is a persistent, runtime-agnostic cognitive layer for distributed AI agents.
## ✨ Features

- ✅ **Event-driven Distillation** – Each store triggers distillation (with fallback timer)
- ✅ **Hierarchical Memory** – Organizing with ltree (root.trading.strategies.scalping)
- ✅ **Temporal Versioning** – Append-only con valid_from/valid_to
- ✅ **Hybrid Retrieval** – Structural + Semantic + Temporal
- ✅ **Multi-tenant Ready** 

## 🚀 Quick Start

```bash
git clone https://github.com/marco-spagn/pcmi.git
cd pcmi
docker compose up -d --build

# Test
curl -X POST http://localhost:8000/v1/memories \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"00000000-0000-0000-0000-000000000000","path":"root.test","content":"Test memory","metadata":{"test":true},"embedding_model":"text-embedding-3-large"}'

curl -X GET "http://localhost:8000/v1/distilled?path_prefix=root.test
