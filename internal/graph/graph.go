package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GraphClient wraps a pgxpool.Pool and issues Cypher queries via Apache AGE.
// All methods degrade gracefully when AGE is not installed.
//
// EXPERIMENTAL — v2.0 spike only.
type GraphClient struct {
	db           *pgxpool.Pool
	queryTimeout time.Duration
}

// NewGraphClient returns a GraphClient backed by db with a default 30s query timeout.
func NewGraphClient(db *pgxpool.Pool) *GraphClient {
	return &GraphClient{db: db, queryTimeout: 30 * time.Second}
}

// SetQueryTimeout overrides the per-query timeout for graph traversals.
// Values <= 0 leave the current timeout unchanged.
func (g *GraphClient) SetQueryTimeout(d time.Duration) {
	if d > 0 {
		g.queryTimeout = d
	}
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

// ageConn acquires a dedicated connection from the pool and sets search_path
// so that AGE operators (e.g. @>) are resolvable.  AGE itself is preloaded via
// shared_preload_libraries (configured in the Docker image).  Caller must
// release the connection via conn.Release().
func (g *GraphClient) ageConn(ctx context.Context) (*pgxpool.Conn, error) {
	if g == nil || g.db == nil {
		return nil, fmt.Errorf("graph: db not initialised")
	}
	conn, err := g.db.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph: acquire conn: %w", err)
	}
	if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public`); err != nil {
		conn.Release()
		return nil, fmt.Errorf("graph: set search_path: %w", err)
	}
	return conn, nil
}

// FindRelated traverses the pcmi_memory_graph starting from memoryID and
// returns nodes reachable within maxDepth hops via any of the given linkTypes.
// When linkTypes is empty all edge types are matched.
// Returns an empty slice (not an error) when AGE is unavailable.
//
// cursor/limit implement keyset pagination over memory IDs.  Pass cursor=0 for
// the first page; subsequent pages pass the last ID from the previous page.
func (g *GraphClient) FindRelated(ctx context.Context, tenantID string, memoryID int64, linkTypes []string, maxDepth int, cursor int64, limit int) (*RelatedResult, error) {
	// Normalise inputs before the AGE check so unit tests can cover these branches.
	if maxDepth < 1 {
		maxDepth = 1
	}
	if limit <= 0 {
		limit = 50
	}
	if cursor < 0 {
		cursor = 0
	}

	if !g.IsAvailable(ctx) {
		return &RelatedResult{Memories: []RelatedMemory{}}, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, g.queryTimeout)
	defer cancel()

	conn, err := g.ageConn(queryCtx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	relPattern, _ := buildRelPattern(maxDepth, "r", nil)
	idStr := fmt.Sprintf("memory.%d", memoryID)
	tenantLiteral := escapeCypherString(tenantID)

	dataQuery := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $$
			MATCH p=(m:Memory {id: '%s', tenant_id: '%s'})-%s->(n:Memory)
			WHERE n.id IS NOT NULL
			RETURN p, n.id, type(r[0]), size(r)
			ORDER BY n.id
		$$) AS (path ag_catalog.agtype, id ag_catalog.agtype, link_type ag_catalog.agtype, depth ag_catalog.agtype)`,
		idStr, tenantLiteral, relPattern,
	)

	rows, err := conn.Query(queryCtx, dataQuery)
	if err != nil {
		return nil, fmt.Errorf("graph FindRelated: %w", err)
	}
	defer rows.Close()

	var all []RelatedMemory
	total := 0
	for rows.Next() {
		var pathRaw, idRaw, ltRaw, depthRaw []byte
		if err := rows.Scan(&pathRaw, &idRaw, &ltRaw, &depthRaw); err != nil {
			return nil, fmt.Errorf("graph FindRelated scan: %w", err)
		}
		if !pathMatchesLinkTypes(pathRaw, linkTypes) {
			continue
		}
		total++
		id, _ := parseMemoryVertexID(string(idRaw))
		if id <= cursor {
			continue
		}
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

	var nextCursor int64
	if len(all) > limit {
		nextCursor = all[limit-1].ID
		all = all[:limit]
	}

	return &RelatedResult{Memories: all, Total: total, NextCursor: nextCursor}, nil
}

// FindChain finds the shortest path between two memories using the given link
// types.  Returns the path as an ordered list of ChainLink entries.
func (g *GraphClient) FindChain(ctx context.Context, tenantID string, fromID, toID int64, linkTypes []string, maxDepth int) (*ChainResult, error) {
	result := &ChainResult{FromID: fromID, ToID: toID}

	// Normalise before the AGE check so unit tests can cover this branch.
	if maxDepth < 1 {
		maxDepth = 3
	}

	if !g.IsAvailable(ctx) {
		return result, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, g.queryTimeout)
	defer cancel()

	conn, err := g.ageConn(queryCtx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	relPattern, _ := buildRelPattern(maxDepth, "e", nil)

	fromStr := fmt.Sprintf("memory.%d", fromID)
	toStr := fmt.Sprintf("memory.%d", toID)
	tenantLiteral := escapeCypherString(tenantID)

	query := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $$
			MATCH p=(a:Memory {id: '%s', tenant_id: '%s'})-%s->(b:Memory {id: '%s', tenant_id: '%s'})
			WHERE a.id IS NOT NULL
			RETURN p, length(p)
			ORDER BY length(p) ASC
		$$) AS (path ag_catalog.agtype, hops ag_catalog.agtype)`,
		fromStr, tenantLiteral, relPattern, toStr, tenantLiteral,
	)

	rows, err := conn.Query(queryCtx, query)
	if err != nil {
		return nil, fmt.Errorf("graph FindChain: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var pathRaw, hopsRaw []byte
		if err := rows.Scan(&pathRaw, &hopsRaw); err != nil {
			return nil, fmt.Errorf("graph FindChain scan: %w", err)
		}
		if !pathMatchesLinkTypes(pathRaw, linkTypes) {
			continue
		}
		result.Connected = true
		result.Hops, _ = strconv.Atoi(strings.Trim(string(hopsRaw), `"`))
		result.Path = parseAGEPath(pathRaw)
		return result, nil
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph FindChain rows: %w", err)
	}

	return result, nil
}

// parseAGEPath parses an AGE agtype path into ChainLink entries.
// AGE represents paths as JSON arrays with ::vertex / ::edge suffixes, e.g.:
//
//	[{...}::vertex, {...}::edge, {...}::vertex]::path
//
// We strip the suffixes before JSON unmarshaling.
func parseAGEPath(raw []byte) []ChainLink {
	// Strip ::vertex, ::edge, and ::path suffixes — they are not valid JSON.
	s := string(raw)
	s = strings.ReplaceAll(s, "::vertex", "")
	s = strings.ReplaceAll(s, "::edge", "")
	s = strings.ReplaceAll(s, "::path", "")
	cleaned := []byte(s)

	var path []json.RawMessage
	if err := json.Unmarshal(cleaned, &path); err != nil || len(path) < 3 {
		return nil
	}

	var links []ChainLink
	for i := 1; i < len(path); i += 2 {
		var edge struct {
			Label      string                     `json:"label"`
			StartID    int64                      `json:"start_id"`
			EndID      int64                      `json:"end_id"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(path[i], &edge); err != nil {
			continue
		}

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
// Tenant scoping is injected automatically.
func (g *GraphClient) ExecuteCypher(ctx context.Context, tenantID, query string) (*CypherResult, error) {
	if !g.IsAvailable(ctx) {
		return nil, fmt.Errorf("cognitive graph not available")
	}

	queryCtx, cancel := context.WithTimeout(ctx, g.queryTimeout)
	defer cancel()

	conn, err := g.ageConn(queryCtx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	trimmed, err := validateCypherQuery(query)
	if err != nil {
		return nil, err
	}

	scopedQuery, err := autoTenantScopeCypher(trimmed, tenantID)
	if err != nil {
		return nil, err
	}

	fullQuery := fmt.Sprintf(
		`SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $$ %s $$) AS (result ag_catalog.agtype)`,
		scopedQuery,
	)

	rows, err := conn.Query(queryCtx, fullQuery)
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

// validateCypherQuery validates a Cypher passthrough query and returns the
// trimmed form.  Only MATCH queries are allowed; write keywords are rejected.
func validateCypherQuery(query string) (string, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "", fmt.Errorf("query must not be empty")
	}
	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "MATCH") {
		return "", fmt.Errorf("only MATCH queries are allowed in Cypher passthrough")
	}
	dangerous := []string{"CREATE ", "DELETE ", "SET ", "REMOVE ", "MERGE ", "DROP ", "CALL ", "LOAD "}
	for _, kw := range dangerous {
		if strings.Contains(upper, kw) {
			return "", fmt.Errorf("keyword %s is not allowed in Cypher passthrough queries", strings.TrimSpace(kw))
		}
	}
	return trimmed, nil
}

// autoTenantScopeCypher injects a tenant_id filter into a read-only Cypher query.
func autoTenantScopeCypher(query, tenantID string) (string, error) {
	alias := ""
	upper := strings.ToUpper(query)
	memIdx := strings.Index(upper, ":MEMORY")
	if memIdx < 0 {
		return "", fmt.Errorf("query must reference at least one :Memory node for tenant scoping")
	}

	parenIdx := strings.LastIndex(query[:memIdx], "(")
	if parenIdx < 0 {
		return "", fmt.Errorf("invalid MATCH pattern: missing ( before :Memory")
	}
	aliasPart := strings.TrimSpace(query[parenIdx+1 : memIdx])
	if idx := strings.IndexAny(aliasPart, " :{"); idx >= 0 {
		alias = aliasPart[:idx]
	} else {
		alias = aliasPart
	}
	if alias == "" {
		return "", fmt.Errorf("could not determine Memory node alias for tenant scoping")
	}

	returnIdx := strings.Index(upper, "RETURN")
	if returnIdx < 0 {
		return "", fmt.Errorf("query must contain a RETURN clause")
	}

	tenantFilter := fmt.Sprintf("%s.tenant_id = '%s'", alias, escapeCypherString(tenantID))

	whereIdx := strings.Index(upper, "WHERE")
	if whereIdx >= 0 && whereIdx < returnIdx {
		existingWhere := strings.TrimSpace(query[whereIdx+5 : returnIdx])
		query = query[:whereIdx+5] + " " + tenantFilter + " AND (" + existingWhere + ") " + query[returnIdx:]
	} else {
		query = query[:returnIdx] + "WHERE " + tenantFilter + " " + query[returnIdx:]
	}

	return query, nil
}

// CreateLink inserts a link into memory_links and syncs it to the AGE graph.
// The DB trigger trg_memory_links_sync_graph handles syncing to the AGE graph.
func (g *GraphClient) CreateLink(ctx context.Context, tenantID string, fromID, toID int64, linkType string, weight float64) error {
	if g == nil || g.db == nil {
		return fmt.Errorf("graph: db not initialised")
	}

	fromPath := fmt.Sprintf("memory.%d", fromID)
	toPath := fmt.Sprintf("memory.%d", toID)

	// Store weight in metadata (weight column is added by migration 019 only when AGE
	// is installed; metadata is always present on memory_links).
	_, err := g.db.Exec(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, metadata)
		VALUES ($1, $2::ltree, $3::ltree, $4, jsonb_build_object('weight', $5::float8))
		ON CONFLICT (tenant_id, from_path, to_path, link_type) DO UPDATE
		    SET metadata = memory_links.metadata || jsonb_build_object('weight', $5::float8)`,
		tenantID, fromPath, toPath, linkType, weight,
	)
	if err != nil {
		return fmt.Errorf("graph CreateLink insert: %w", err)
	}

	return nil
}

// SyncMemoryLink merges a memory_links row into the AGE graph (best-effort).
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

// buildRelPattern builds the Cypher relationship pattern and WHERE type filter
// for graph traversals.  relVar is the edge variable name ("r" or "e").
func buildRelPattern(maxDepth int, relVar string, linkTypes []string) (string, string) {
	return fmt.Sprintf("[%s*1..%d]", relVar, maxDepth), ""
}

// buildCursorClause returns a Cypher WHERE fragment for keyset pagination.
// Returns empty string when cursor is 0 or negative.
func buildCursorClause(cursor int64) string {
	if cursor > 0 {
		return fmt.Sprintf(" AND n.id > 'memory.%d'", cursor)
	}
	return ""
}

func parseMemoryVertexID(raw string) (int64, error) {
	raw = strings.Trim(raw, `"`)
	raw = strings.TrimPrefix(raw, "memory.")
	return strconv.ParseInt(raw, 10, 64)
}

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

func escapeCypherString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

func pathMatchesLinkTypes(raw []byte, linkTypes []string) bool {
	if len(linkTypes) == 0 {
		return true
	}
	allowed := make(map[string]struct{}, len(linkTypes))
	for _, lt := range linkTypes {
		if sanitized := sanitizeLinkType(lt); sanitized != "" {
			allowed[sanitized] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return true
	}
	links := parseAGEPath(raw)
	if len(links) == 0 {
		return false
	}
	for _, link := range links {
		if _, ok := allowed[link.LinkType]; !ok {
			return false
		}
	}
	return true
}
