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

// RelatedResult holds paginated traversal results.
type RelatedResult struct {
	Memories []RelatedMemory `json:"-"`
	Total    int             `json:"-"`
}

// ChainLink is one hop in a reconstructed path between two memories.
type ChainLink struct {
	FromID   int64  `json:"from_id"`
	ToID     int64  `json:"to_id"`
	LinkType string `json:"link_type"`
	HopIndex int    `json:"hop"`
}

// ChainResult is the result of a chain-finding query.
type ChainResult struct {
	FromID    int64       `json:"from_id"`
	ToID      int64       `json:"to_id"`
	Connected bool        `json:"connected"`
	Hops      int         `json:"hops"`
	Path      []ChainLink `json:"path"`
}

// CypherResult holds the result of a Cypher passthrough query.
type CypherResult struct {
	Columns []string                 `json:"columns"`
	Rows    []map[string]interface{} `json:"rows"`
}
