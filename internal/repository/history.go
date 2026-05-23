package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/model"
)

// ListPathHistory returns all versions for a path, newest version first.
func (r *MemoryRepository) ListPathHistory(ctx context.Context, tenantID, path string, limit int) ([]model.MemoryEntry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := `
		SELECT ` + memoryEntrySelectCols + `,
		       NULL::float8 AS relevance_score
		FROM memory_entries
		WHERE tenant_id = $1::uuid AND path = $2::ltree
		ORDER BY version DESC
		LIMIT $3`

	rows, err := r.r.Query(ctx, q, tenantID, path, limit)
	if err != nil {
		return nil, fmt.Errorf("list path history: %w", err)
	}
	defer rows.Close()

	var entries []model.MemoryEntry
	for rows.Next() {
		e, scanErr := r.scanMemoryEntry(rows, true)
		if scanErr != nil {
			return nil, fmt.Errorf("history scan: %w", scanErr)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
