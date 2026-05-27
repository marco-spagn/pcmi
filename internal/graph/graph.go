package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GraphClient wraps a pgxpool.Pool and issues Cypher queries via Apache AGE.
// All methods degrade gracefully when AGE is not installed.
//
// EXPERIMENTAL — v2.0 spike only.
type GraphClient struct {
	db *pgxpool.Pool
}

// NewGraphClient returns a GraphClient backed by db.
func NewGraphClient(db *pgxpool.Pool) *GraphClient {
	return &GraphClient{db: db}
}

// IsAvailable reports whether the Apache AGE extension is installed and the
// pcmi_memory_graph exists.  Returns false on any error.
func (g *GraphClient) IsAvailable(ctx context.Context) bool {
	if g == nil || g.db == nil {
		return false
	}
	rows, err := g.db.Query(ctx, "SELECT * FROM ag_catalog.ag_graph LIMIT 1")
	if err != nil {
		return false
	}
	rows.Close()
	return rows.Err() == nil
}

// FindRelated traverses the pcmi_memory_graph starting from memoryID and
// returns nodes reachable within maxDepth hops via any of the given linkTypes.
// When linkTypes is empty all edge types are matched.
// Returns an empty slice (not an error) when AGE is unavailable.
//
// offset/limit enable pagination (applied in-memory for now).
// Total is the count before pagination.
func (g *GraphClient) FindRelated(ctx context.Context, tenantID string, memoryID int64, linkTypes []string, maxDepth, offset, limit int) (*RelatedResult, error) {
	if !g.IsAvailable(ctx) {
		return &RelatedResult{Memories: []RelatedMemory{}}, nil
	}
	if maxDepth < 1 {
		maxDepth = 1
	}

	// Build the Cypher path pattern, optionally filtering by relationship type.
	relPattern := fmt.Sprintf("[r*1..%d]", maxDepth)
	if len(linkTypes) > 0 {
		quoted := make([]string, len(linkTypes))
		for i, lt := range linkTypes {
			quoted[i] = sanitizeLinkType(lt)
		}
		relPattern = fmt.Sprintf("[r:%s*1..%d]", strings.Join(quoted, "|"), maxDepth)
	}

	// Vertex id matches sync_memory_link_to_graph: ltree path "memory.<id>".
	idStr := fmt.Sprintf("memory.%d", memoryID)

	query := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
			MATCH (m:Memory {id: '%s', tenant_id: '%s'})-`+relPattern+`->(n:Memory)
			RETURN n.id, type(r[0]), length(r)
		$cypher$) AS (id ag_catalog.agtype, link_type ag_catalog.agtype, depth ag_catalog.agtype)`,
		idStr, tenantID,
	)

	rows, err := g.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("graph FindRelated: %w", err)
	}
	defer rows.Close()

	var all []RelatedMemory
	for rows.Next() {
		var idRaw, ltRaw, depthRaw []byte
		if err := rows.Scan(&idRaw, &ltRaw, &depthRaw); err != nil {
			return nil, fmt.Errorf("graph FindRelated scan: %w", err)
		}
		id, _ := parseMemoryVertexID(string(idRaw))
		depth, _ := strconv.Atoi(strings.Trim(string(depthRaw), `"`))
		all = append(all, RelatedMemory{
			ID:       id,
			LinkType: strings.Trim(string(ltRaw), `"`),
			Depth:    depth,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph FindRelated rows: %w", err)
	}

	total := len(all)
	if offset > 0 {
		if offset >= len(all) {
			return &RelatedResult{Memories: []RelatedMemory{}, Total: total}, nil
		}
		all = all[offset:]
	}
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}

	return &RelatedResult{Memories: all, Total: total}, nil
}

// FindChain finds the shortest path between two memories using the given link
// types.  Returns the path as an ordered list of ChainLink entries.
func (g *GraphClient) FindChain(ctx context.Context, tenantID string, fromID, toID int64, linkTypes []string, maxDepth int) (*ChainResult, error) {
	result := &ChainResult{FromID: fromID, ToID: toID}

	if !g.IsAvailable(ctx) {
		return result, nil
	}
	if maxDepth < 1 {
		maxDepth = 3
	}

	relPattern := fmt.Sprintf("[e*1..%d]", maxDepth)
	if len(linkTypes) > 0 {
		quoted := make([]string, len(linkTypes))
		for i, lt := range linkTypes {
			quoted[i] = sanitizeLinkType(lt)
		}
		relPattern = fmt.Sprintf("[e:%s*1..%d]", strings.Join(quoted, "|"), maxDepth)
	}

	fromStr := fmt.Sprintf("memory.%d", fromID)
	toStr := fmt.Sprintf("memory.%d", toID)

	query := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$
			MATCH p=(a:Memory {id: '%s', tenant_id: '%s'})-%s->(b:Memory {id: '%s', tenant_id: '%s'})
			RETURN p, length(p)
			ORDER BY length(p) ASC
			LIMIT 1
		$cypher$) AS (path ag_catalog.agtype, hops ag_catalog.agtype)`,
		fromStr, tenantID, relPattern, toStr, tenantID,
	)

	rows, err := g.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("graph FindChain: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return result, nil
	}

	var pathRaw, hopsRaw []byte
	if err := rows.Scan(&pathRaw, &hopsRaw); err != nil {
		return nil, fmt.Errorf("graph FindChain scan: %w", err)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("graph FindChain rows: %w", err)
	}

	result.Connected = true
	result.Hops, _ = strconv.Atoi(strings.Trim(string(hopsRaw), `"`))

	// Parse the AGE path: a JSON array alternating vertices and edges.
	// [v0, e0, v1, e1, ..., vN]
	chainLinks := parseAGEPath(pathRaw)
	result.Path = chainLinks

	return result, nil
}

// parseAGEPath parses an AGE agtype path into ChainLink entries.
// The path format is a JSON array: [vertex, edge, vertex, edge, ..., vertex].
func parseAGEPath(raw []byte) []ChainLink {
	var path []json.RawMessage
	if err := json.Unmarshal(raw, &path); err != nil || len(path) < 3 {
		return nil
	}

	var links []ChainLink
	for i := 1; i < len(path); i += 2 {
		var edge struct {
			Label    string `json:"label"`
			StartID  int64  `json:"start_id"`
			EndID    int64  `json:"end_id"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(path[i], &edge); err != nil {
			continue
		}

		// Parse vertex IDs from the surrounding vertices.
		var prevV, nextV struct {
			Properties struct {
				ID string `json:"id"`
			} `json:"properties"`
		}
		fromID := int64(0)
		toID := int64(0)
		if i > 0 {
			if err := json.Unmarshal(path[i-1], &prevV); err == nil {
				fromID, _ = parseMemoryVertexID(prevV.Properties.ID)
			}
		}
		if i+1 < len(path) {
			if err := json.Unmarshal(path[i+1], &nextV); err == nil {
				toID, _ = parseMemoryVertexID(nextV.Properties.ID)
			}
		}

		hop := (i - 1) / 2
		links = append(links, ChainLink{
			FromID:   fromID,
			ToID:     toID,
			LinkType: edge.Label,
			HopIndex: hop,
		})
	}
	return links
}

// ExecuteCypher runs a read-only Cypher query against pcmi_memory_graph.
// The query must start with MATCH and must not contain write keywords.
// Tenant scoping is the caller's responsibility — include tenant_id in the query.
func (g *GraphClient) ExecuteCypher(ctx context.Context, tenantID, query string) (*CypherResult, error) {
	if !g.IsAvailable(ctx) {
		return nil, fmt.Errorf("cognitive graph not available")
	}

	upper := strings.ToUpper(strings.TrimSpace(query))
	if !strings.HasPrefix(upper, "MATCH") {
		return nil, fmt.Errorf("only MATCH queries are allowed in Cypher passthrough")
	}
	dangerous := []string{"CREATE ", "DELETE ", "SET ", "REMOVE ", "MERGE ", "DROP ", "CALL ", "LOAD "}
	for _, kw := range dangerous {
		if strings.Contains(upper, kw) {
			return nil, fmt.Errorf("keyword %s is not allowed in Cypher passthrough queries", strings.TrimSpace(kw))
		}
	}

	fullQuery := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $cypher$ %s $cypher$) AS (result ag_catalog.agtype)`,
		query,
	)

	rows, err := g.db.Query(ctx, fullQuery)
	if err != nil {
		return nil, fmt.Errorf("graph ExecuteCypher: %w", err)
	}
	defer rows.Close()

	var jsonRows []map[string]interface{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("graph ExecuteCypher scan: %w", err)
		}
		var parsed interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			parsed = string(raw)
		}
		jsonRows = append(jsonRows, map[string]interface{}{"result": parsed})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph ExecuteCypher rows: %w", err)
	}

	return &CypherResult{
		Columns: []string{"result"},
		Rows:    jsonRows,
	}, nil
}

// CreateLink inserts a link into memory_links and syncs it to the AGE graph.
// fromID and toID are memory_entries.id values.
// The graph vertex id property is set to the string form of the integer ID so
// that FindRelated can parse it back.
func (g *GraphClient) CreateLink(ctx context.Context, tenantID string, fromID, toID int64, linkType string, weight float64) error {
	if g == nil || g.db == nil {
		return fmt.Errorf("graph: db not initialised")
	}

	fromPath := fmt.Sprintf("memory.%d", fromID)
	toPath := fmt.Sprintf("memory.%d", toID)

	_, err := g.db.Exec(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, weight, metadata)
		VALUES ($1, $2::ltree, $3::ltree, $4, $5, '{}')
		ON CONFLICT (tenant_id, from_path, to_path, link_type) DO UPDATE
		    SET weight = EXCLUDED.weight`,
		tenantID, fromPath, toPath, linkType, weight,
	)
	if err != nil {
		return fmt.Errorf("graph CreateLink insert: %w", err)
	}

	g.SyncMemoryLink(ctx, tenantID, fromPath, toPath, linkType, weight)
	return nil
}

// SyncMemoryLink merges a memory_links row into the AGE graph (best-effort).
// No-op when AGE is unavailable. Used by CreateLink and safe to call after
// repository upserts when triggers are not yet applied.
func (g *GraphClient) SyncMemoryLink(ctx context.Context, tenantID, fromPath, toPath, linkType string, weight float64) {
	if g == nil || g.db == nil || !g.IsAvailable(ctx) {
		return
	}
	if weight <= 0 {
		weight = 1.0
	}
	_, _ = g.db.Exec(ctx,
		`SELECT public.sync_memory_link_to_graph($1, $2, $3, $4, $5)`,
		fromPath, toPath, sanitizeLinkType(linkType), weight, tenantID,
	)
}

// parseMemoryVertexID extracts memory_entries.id from an AGE vertex id property
// stored as "memory.<id>" (matches sync_memory_link_to_graph / ltree paths).
func parseMemoryVertexID(raw string) (int64, error) {
	raw = strings.Trim(raw, `"`)
	raw = strings.TrimPrefix(raw, "memory.")
	return strconv.ParseInt(raw, 10, 64)
}

// sanitizeLinkType strips non-word characters so the string is safe to embed
// as a Cypher relationship label.
func sanitizeLinkType(lt string) string {
	var b strings.Builder
	for _, r := range lt {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}
