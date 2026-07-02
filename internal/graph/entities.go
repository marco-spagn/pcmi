package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/marco-spagn/pcmi/internal/extraction"
)

// EntityMention is one promoted slot linked from a memory in AGE.
type EntityMention struct {
	Slot       string  `json:"slot"`
	Kind       string  `json:"kind"`
	Key        string  `json:"key"`
	Confidence float64 `json:"confidence,omitempty"`
}

// EntityMemory is a memory that mentions an entity (or shares one).
type EntityMemory struct {
	ID         int64   `json:"id"`
	SharedKind string  `json:"shared_kind,omitempty"`
	SharedKey  string  `json:"shared_key,omitempty"`
	Slot       string  `json:"slot,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// EntityMemoriesResult paginates memories found via entity traversal.
type EntityMemoriesResult struct {
	Memories   []EntityMemory
	Total      int
	NextCursor int64
}

// ReconcileEntityMentions replaces mentions edges for a memory in AGE.
func (g *GraphClient) ReconcileEntityMentions(ctx context.Context, tenantID string, memoryID int64, memoryVersion int, mentions []EntityMention) error {
	if g == nil || g.db == nil {
		return fmt.Errorf("graph: db not initialised")
	}
	if !g.IsAvailable(ctx) {
		return nil
	}
	payload, err := json.Marshal(mentions)
	if err != nil {
		return fmt.Errorf("graph ReconcileEntityMentions marshal: %w", err)
	}
	memVertex := fmt.Sprintf("memory.%d", memoryID)
	_, err = g.db.Exec(ctx,
		`SELECT public.reconcile_entity_mentions_for_memory($1, $2::uuid, $3, $4::jsonb)`,
		memVertex, tenantID, memoryVersion, payload,
	)
	if err != nil {
		return fmt.Errorf("graph ReconcileEntityMentions: %w", err)
	}
	return nil
}

// SyncPromotedEntities is a convenience wrapper for extraction promotion.
func (g *GraphClient) SyncPromotedEntities(ctx context.Context, tenantID string, memoryID int64, memoryVersion int, profile *extraction.Profile, rec *extraction.Record) error {
	promoted := extraction.PromoteEntities(profile, rec)
	mentions := make([]EntityMention, 0, len(promoted))
	for _, p := range promoted {
		mentions = append(mentions, EntityMention{
			Slot:       p.Slot,
			Kind:       p.Kind,
			Key:        p.Key,
			Confidence: p.Confidence,
		})
	}
	return g.ReconcileEntityMentions(ctx, tenantID, memoryID, memoryVersion, mentions)
}

// FindEntitiesForMemory lists entity vertices mentioned by a memory.
func (g *GraphClient) FindEntitiesForMemory(ctx context.Context, tenantID string, memoryID int64) ([]EntityMention, error) {
	if !g.IsAvailable(ctx) {
		return []EntityMention{}, nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, g.queryTimeout)
	defer cancel()

	conn, err := g.ageConn(queryCtx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	idStr := fmt.Sprintf("memory.%d", memoryID)
	tenantLiteral := escapeCypherString(tenantID)

	dataQuery := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $$
			MATCH (m:Memory {id: '%s', tenant_id: '%s'})-[r:mentions]->(e:Entity)
			RETURN e.kind, e.key, r.slot, r.confidence
			ORDER BY e.kind, e.key
		$$) AS (kind ag_catalog.agtype, key ag_catalog.agtype, slot ag_catalog.agtype, confidence ag_catalog.agtype)`,
		idStr, tenantLiteral,
	)

	rows, err := conn.Query(queryCtx, dataQuery)
	if err != nil {
		return nil, fmt.Errorf("graph FindEntitiesForMemory: %w", err)
	}
	defer rows.Close()

	var out []EntityMention
	for rows.Next() {
		var kindRaw, keyRaw, slotRaw, confRaw []byte
		if err := rows.Scan(&kindRaw, &keyRaw, &slotRaw, &confRaw); err != nil {
			return nil, fmt.Errorf("graph FindEntitiesForMemory scan: %w", err)
		}
		conf, _ := strconv.ParseFloat(strings.Trim(string(confRaw), `"`), 64)
		out = append(out, EntityMention{
			Kind:       strings.Trim(string(kindRaw), `"`),
			Key:        strings.Trim(string(keyRaw), `"`),
			Slot:       strings.Trim(string(slotRaw), `"`),
			Confidence: conf,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph FindEntitiesForMemory rows: %w", err)
	}
	if out == nil {
		out = []EntityMention{}
	}
	return out, nil
}

// FindMemoriesByEntity returns memories that mention the same entity kind+key.
func (g *GraphClient) FindMemoriesByEntity(ctx context.Context, tenantID, kind, key string, cursor int64, limit int) (*EntityMemoriesResult, error) {
	kind = strings.TrimSpace(kind)
	key = strings.TrimSpace(key)
	if kind == "" || key == "" {
		return nil, fmt.Errorf("kind and key are required")
	}
	if limit <= 0 {
		limit = 50
	}
	if cursor < 0 {
		cursor = 0
	}
	if !g.IsAvailable(ctx) {
		return &EntityMemoriesResult{Memories: []EntityMemory{}}, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, g.queryTimeout)
	defer cancel()

	conn, err := g.ageConn(queryCtx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	tenantLiteral := escapeCypherString(tenantID)
	kindLiteral := escapeCypherString(kind)
	keyLiteral := escapeCypherString(key)

	dataQuery := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $$
			MATCH (e:Entity {kind: '%s', key: '%s', tenant_id: '%s'})<-[r:mentions]-(m:Memory)
			WHERE m.id IS NOT NULL
			RETURN m.id, r.slot, r.confidence
		$$) AS (id ag_catalog.agtype, slot ag_catalog.agtype, confidence ag_catalog.agtype)`,
		kindLiteral, keyLiteral, tenantLiteral,
	)

	rows, err := conn.Query(queryCtx, dataQuery)
	if err != nil {
		return nil, fmt.Errorf("graph FindMemoriesByEntity: %w", err)
	}
	defer rows.Close()

	seen := make(map[int64]EntityMemory)
	for rows.Next() {
		var idRaw, slotRaw, confRaw []byte
		if err := rows.Scan(&idRaw, &slotRaw, &confRaw); err != nil {
			return nil, fmt.Errorf("graph FindMemoriesByEntity scan: %w", err)
		}
		id, err := parseMemoryVertexID(string(idRaw))
		if err != nil || id <= 0 {
			continue
		}
		conf, _ := strconv.ParseFloat(strings.Trim(string(confRaw), `"`), 64)
		candidate := EntityMemory{
			ID:         id,
			SharedKind: kind,
			SharedKey:  key,
			Slot:       strings.Trim(string(slotRaw), `"`),
			Confidence: conf,
		}
		if existing, ok := seen[id]; !ok || candidate.Confidence > existing.Confidence {
			seen[id] = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph FindMemoriesByEntity rows: %w", err)
	}

	return paginateEntityMemories(seen, cursor, limit), nil
}

// FindRelatedViaEntity returns other memories sharing any entity with memoryID.
func (g *GraphClient) FindRelatedViaEntity(ctx context.Context, tenantID string, memoryID int64, cursor int64, limit int) (*EntityMemoriesResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if cursor < 0 {
		cursor = 0
	}
	if !g.IsAvailable(ctx) {
		return &EntityMemoriesResult{Memories: []EntityMemory{}}, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, g.queryTimeout)
	defer cancel()

	conn, err := g.ageConn(queryCtx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	idStr := fmt.Sprintf("memory.%d", memoryID)
	tenantLiteral := escapeCypherString(tenantID)

	dataQuery := fmt.Sprintf(`
		SELECT * FROM ag_catalog.cypher('pcmi_memory_graph', $$
			MATCH (m1:Memory {id: '%s', tenant_id: '%s'})-[r1:mentions]->(e:Entity)<-[r2:mentions]-(m2:Memory)
			WHERE m2.id <> m1.id AND m2.tenant_id = '%s'
			RETURN m2.id, e.kind, e.key, r2.slot, r2.confidence
		$$) AS (id ag_catalog.agtype, kind ag_catalog.agtype, key ag_catalog.agtype, slot ag_catalog.agtype, confidence ag_catalog.agtype)`,
		idStr, tenantLiteral, tenantLiteral,
	)

	rows, err := conn.Query(queryCtx, dataQuery)
	if err != nil {
		return nil, fmt.Errorf("graph FindRelatedViaEntity: %w", err)
	}
	defer rows.Close()

	seen := make(map[int64]EntityMemory)
	for rows.Next() {
		var idRaw, kindRaw, keyRaw, slotRaw, confRaw []byte
		if err := rows.Scan(&idRaw, &kindRaw, &keyRaw, &slotRaw, &confRaw); err != nil {
			return nil, fmt.Errorf("graph FindRelatedViaEntity scan: %w", err)
		}
		id, err := parseMemoryVertexID(string(idRaw))
		if err != nil || id <= 0 || id == memoryID {
			continue
		}
		conf, _ := strconv.ParseFloat(strings.Trim(string(confRaw), `"`), 64)
		candidate := EntityMemory{
			ID:         id,
			SharedKind: strings.Trim(string(kindRaw), `"`),
			SharedKey:  strings.Trim(string(keyRaw), `"`),
			Slot:       strings.Trim(string(slotRaw), `"`),
			Confidence: conf,
		}
		if existing, ok := seen[id]; !ok || candidate.Confidence > existing.Confidence {
			seen[id] = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph FindRelatedViaEntity rows: %w", err)
	}

	return paginateEntityMemories(seen, cursor, limit), nil
}

func paginateEntityMemories(seen map[int64]EntityMemory, cursor int64, limit int) *EntityMemoriesResult {
	all := make([]EntityMemory, 0, len(seen))
	for id, mem := range seen {
		if id <= cursor {
			continue
		}
		all = append(all, mem)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})
	total := len(seen)
	var nextCursor int64
	if len(all) > limit {
		nextCursor = all[limit-1].ID
		all = all[:limit]
	}
	if all == nil {
		all = []EntityMemory{}
	}
	return &EntityMemoriesResult{Memories: all, Total: total, NextCursor: nextCursor}
}
