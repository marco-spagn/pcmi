package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/model"
)

// ExportMemories returns current memories under path_prefix for tenant migration.
func (r *MemoryRepository) ExportMemories(ctx context.Context, tenantID, pathPrefix string, limit int, includeEmb bool) ([]model.MemoryEntry, error) {
	path := strings.TrimSpace(pathPrefix)
	if path == "" {
		path = "root"
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}

	embCol := "NULL::vector AS embedding"
	if includeEmb {
		embCol = "embedding"
	}

	q := fmt.Sprintf(`
		SELECT id, tenant_id, path, content, metadata, tags, %s, embedding_model, embedding_space,
		       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at, content_encrypted,
		       importance, access_count, last_accessed_at,
		       NULL::float8 AS relevance_score
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND path <@ $2::ltree
		  AND valid_to IS NULL
		ORDER BY path, version DESC
		LIMIT $3`, embCol)

	rows, err := r.r.Query(ctx, q, tenantID, path, limit)
	if err != nil {
		return nil, fmt.Errorf("export memories: %w", err)
	}
	defer rows.Close()

	var entries []model.MemoryEntry
	for rows.Next() {
		e, scanErr := r.scanMemoryEntry(rows, true)
		if scanErr != nil {
			return nil, fmt.Errorf("export scan: %w", scanErr)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
