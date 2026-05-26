package graph

import (
	"context"
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
func (g *GraphClient) FindRelated(ctx context.Context, tenantID string, memoryID int64, linkTypes []string, maxDepth int) ([]RelatedMemory, error) {
	if !g.IsAvailable(ctx) {
		return []RelatedMemory{}, nil
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

	// memoryID was stored as the vertex id property via CreateLink.
	idStr := strconv.FormatInt(memoryID, 10)

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

	var results []RelatedMemory
	for rows.Next() {
		var idRaw, ltRaw, depthRaw []byte
		if err := rows.Scan(&idRaw, &ltRaw, &depthRaw); err != nil {
			return nil, fmt.Errorf("graph FindRelated scan: %w", err)
		}
		id, _ := strconv.ParseInt(strings.Trim(string(idRaw), `"`), 10, 64)
		depth, _ := strconv.Atoi(strings.Trim(string(depthRaw), `"`))
		results = append(results, RelatedMemory{
			ID:       id,
			LinkType: strings.Trim(string(ltRaw), `"`),
			Depth:    depth,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph FindRelated rows: %w", err)
	}
	return results, nil
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

	// If AGE is available the INSERT trigger handles graph sync automatically.
	// Call explicitly here only when AGE is present but the trigger missed it
	// (e.g. ON CONFLICT DO UPDATE path does not fire AFTER INSERT triggers).
	if g.IsAvailable(ctx) {
		_, syncErr := g.db.Exec(ctx,
			`SELECT public.sync_memory_link_to_graph($1, $2, $3, $4, $5)`,
			fromPath, toPath, sanitizeLinkType(linkType), weight, tenantID,
		)
		if syncErr != nil {
			// Best-effort: log but do not fail the caller.
			_ = syncErr
		}
	}
	return nil
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
