// Package graph provides an experimental Apache AGE client for v2.0 Cognitive Graph.
// SPIKE — not production-ready.
package graph

const (
	LinkTypeCausal      = "causal"
	LinkTypeTemporal    = "temporal"
	LinkTypeContradicts = "contradicts"
	LinkTypeSupports    = "supports"
	LinkTypeRelated     = "related"
)

// RelatedMemory is a node reachable from a source memory via graph traversal.
type RelatedMemory struct {
	// ID is the memory_entries.id of the related node (parsed from vertex path).
	ID       int64  `json:"id"`
	LinkType string `json:"link_type"`
	Depth    int    `json:"depth"`
}
