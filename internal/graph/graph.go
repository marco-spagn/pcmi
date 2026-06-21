package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

const graphAvailabilityQuery = "SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'pcmi_memory_graph' LIMIT 1"

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
	rows, err := g.db.Query(ctx, graphAvailabilityQuery)
	if err != nil {
		return false
	}
	defer rows.Close()
	if !rows.Next() {
		return false
	}
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
			WHERE n.id IS NOT NULL AND n.tenant_id = '%s'
			RETURN p, n.id, type(r[0]), size(r)
		$$) AS (path ag_catalog.agtype, id ag_catalog.agtype, link_type ag_catalog.agtype, depth ag_catalog.agtype)`,
		idStr, tenantLiteral, relPattern, tenantLiteral,
	)

	rows, err := conn.Query(queryCtx, dataQuery)
	if err != nil {
		return nil, fmt.Errorf("graph FindRelated: %w", err)
	}
	defer rows.Close()

	var all []RelatedMemory
	seen := make(map[int64]RelatedMemory)
	total := 0
	for rows.Next() {
		var pathRaw, idRaw, ltRaw, depthRaw []byte
		if err := rows.Scan(&pathRaw, &idRaw, &ltRaw, &depthRaw); err != nil {
			return nil, fmt.Errorf("graph FindRelated scan: %w", err)
		}
		if !pathMatchesLinkTypes(pathRaw, linkTypes) {
			continue
		}
		id, _ := parseMemoryVertexID(string(idRaw))
		depth, _ := strconv.Atoi(strings.Trim(string(depthRaw), `"`))
		candidate := RelatedMemory{
			ID:       id,
			LinkType: strings.Trim(string(ltRaw), `"`),
			Depth:    depth,
		}
		if existing, ok := seen[id]; !ok || candidate.Depth < existing.Depth || (candidate.Depth == existing.Depth && candidate.LinkType < existing.LinkType) {
			seen[id] = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph FindRelated rows: %w", err)
	}

	for _, memory := range seen {
		if memory.ID <= cursor {
			continue
		}
		all = append(all, memory)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})
	total = len(seen)

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
	if strings.Contains(trimmed, "$$") {
		return "", fmt.Errorf("dollar-quote delimiters are not allowed in Cypher passthrough queries")
	}
	if strings.Contains(trimmed, ";") {
		return "", fmt.Errorf("multiple statements are not allowed in Cypher passthrough queries")
	}
	upper := strings.ToUpper(trimmed)
	if !strings.HasPrefix(upper, "MATCH") {
		return "", fmt.Errorf("only MATCH queries are allowed in Cypher passthrough")
	}
	dangerous := map[string]struct{}{
		"ALTER": {}, "CALL": {}, "COPY": {}, "CREATE": {}, "DELETE": {}, "DROP": {}, "GRANT": {},
		"INSERT": {}, "LOAD": {}, "MERGE": {}, "REMOVE": {}, "REVOKE": {}, "SET": {}, "TRUNCATE": {},
		"UPDATE": {}, "VACUUM": {},
	}
	// Multi-part and set-combination keywords are rejected because automatic
	// tenant scoping (autoTenantScopeCypher) injects a single tenant filter before
	// the FIRST RETURN clause. A UNION's second sub-query has its own RETURN that
	// would never be scoped, leaking other tenants' nodes (e.g.
	// "MATCH (m:Memory) RETURN m.id UNION MATCH (m:Memory) RETURN m.id" compiles
	// and returns every tenant's ids). WITH/UNWIND/FOREACH re-scope or introduce
	// variables the single-injection model cannot reason about, so they are also
	// rejected rather than silently mis-scoped.
	unsupported := map[string]struct{}{
		"UNION": {}, "WITH": {}, "UNWIND": {}, "FOREACH": {},
	}
	for _, token := range cypherKeywordTokens(upper) {
		if _, ok := dangerous[token]; ok {
			return "", fmt.Errorf("keyword %s is not allowed in Cypher passthrough queries", token)
		}
		if _, ok := unsupported[token]; ok {
			return "", fmt.Errorf("keyword %s is not supported in Cypher passthrough queries: it breaks automatic tenant scoping", token)
		}
	}
	return trimmed, nil
}

func cypherKeywordTokens(query string) []string {
	return strings.FieldsFunc(query, func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_'
	})
}

// autoTenantScopeCypher injects a tenant_id filter into a read-only Cypher query.
func autoTenantScopeCypher(query, tenantID string) (string, error) {
	upper := strings.ToUpper(query)
	aliases, err := memoryAliases(query)
	if err != nil {
		return "", err
	}

	returnIdx := strings.Index(upper, "RETURN")
	if returnIdx < 0 {
		return "", fmt.Errorf("query must contain a RETURN clause")
	}

	tenantLiteral := escapeCypherString(tenantID)
	filters := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		filters = append(filters, fmt.Sprintf("%s.tenant_id = '%s'", alias, tenantLiteral))
	}
	tenantFilter := strings.Join(filters, " AND ")

	whereIdx := strings.Index(upper, "WHERE")
	if whereIdx >= 0 && whereIdx < returnIdx {
		existingWhere := strings.TrimSpace(query[whereIdx+5 : returnIdx])
		query = query[:whereIdx+5] + " " + tenantFilter + " AND (" + existingWhere + ") " + query[returnIdx:]
	} else {
		query = query[:returnIdx] + "WHERE " + tenantFilter + " " + query[returnIdx:]
	}

	return query, nil
}

func memoryAliases(query string) ([]string, error) {
	upper := strings.ToUpper(query)
	seen := make(map[string]struct{})
	var aliases []string
	offset := 0
	for {
		relIdx := strings.Index(upper[offset:], ":MEMORY")
		if relIdx < 0 {
			break
		}
		memIdx := offset + relIdx
		parenIdx := strings.LastIndex(query[:memIdx], "(")
		if parenIdx < 0 {
			return nil, fmt.Errorf("invalid MATCH pattern: missing ( before :Memory")
		}
		aliasPart := strings.TrimSpace(query[parenIdx+1 : memIdx])
		if idx := strings.IndexAny(aliasPart, " :{"); idx >= 0 {
			aliasPart = aliasPart[:idx]
		}
		alias := strings.TrimSpace(aliasPart)
		if alias == "" {
			return nil, fmt.Errorf("could not determine Memory node alias for tenant scoping")
		}
		if _, ok := seen[alias]; !ok {
			seen[alias] = struct{}{}
			aliases = append(aliases, alias)
		}
		offset = memIdx + len(":MEMORY")
	}
	if len(aliases) == 0 {
		return nil, fmt.Errorf("query must reference at least one :Memory node for tenant scoping")
	}
	return aliases, nil
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
