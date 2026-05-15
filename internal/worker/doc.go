// Package worker raggruppa i job in background: embedding, distillation, pruning, consolidation, expiry.
// Sono attivati da cmd/worker e sottoscrivono Redis (memory.stored, memory.updated, memory.refine.requested).
// Richiedono DATABASE_URL e, per LLM/embeddings, variabili OpenAI coerenti con l’API.
package worker
