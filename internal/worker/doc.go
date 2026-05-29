// Package worker groups background jobs: embedding, distillation, pruning, consolidation, expiry.
// They are started by cmd/worker and subscribe to Redis (memory.stored, memory.updated, memory.refine.requested).
// Require DATABASE_URL and, for LLM/embeddings, OpenAI variables consistent with the API.
package worker
