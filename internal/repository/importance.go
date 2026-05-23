package repository

import (
	"context"
	"fmt"
	"strings"
)

// UpdateImportance sets importance on the current (valid_to IS NULL) row for path.
func (r *MemoryRepository) UpdateImportance(ctx context.Context, tenantID, path string, importance float64) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is required")
	}
	tag, err := r.w.Exec(ctx, `
		UPDATE memory_entries
		SET importance = $3
		WHERE tenant_id = $1::uuid AND path = $2::ltree AND valid_to IS NULL`,
		tenantID, path, importance)
	if err != nil {
		return fmt.Errorf("update importance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("memory not found for path %s", path)
	}
	return nil
}

// touchRetrievedMemories increments access_count and last_accessed_at for returned rows.
func (r *MemoryRepository) touchRetrievedMemories(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.w.Exec(ctx, `
		UPDATE memory_entries
		SET access_count = access_count + 1,
		    last_accessed_at = NOW()
		WHERE id = ANY($1::bigint[])`, ids)
	if err != nil {
		return fmt.Errorf("touch retrieved memories: %w", err)
	}
	return nil
}
