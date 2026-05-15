// Package handler registra le route Fiber per l’API REST PCMI: memorie, retrieve, eventi (SSE),
// webhooks, admin, batch, lineage, links, stats e refine. I tenant e i ruoli arrivano da middleware;
// la logica pesante è delegata a internal/service e internal/repository.
//
// Convenzione: le route GET più specifiche (es. /lineage/*, /memories/history) devono essere
// registrate prima del wildcard GET /memories/* per evitare che Fiber interpreti "lineage" come path.
package handler
