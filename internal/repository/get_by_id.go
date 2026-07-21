package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/marco-spagn/pcmi/internal/model"
)

// GetByIDResolveCurrent loads a memory by graph vertex id. When the id refers to a
// superseded row (stale AGE vertex), the current version at the same path is returned.
func (r *MemoryRepository) GetByIDResolveCurrent(ctx context.Context, tenantID string, memoryID int64) (*model.MemoryEntry, int64, error) {
	if memoryID <= 0 {
		return nil, 0, fmt.Errorf("memory id must be positive")
	}

	currentQ := `
		SELECT ` + memoryEntrySelectCols + `
		FROM memory_entries
		WHERE tenant_id = $1::uuid AND id = $2 AND valid_to IS NULL
		LIMIT 1`
	row := r.r.QueryRow(ctx, currentQ, tenantID, memoryID)
	entry, err := r.scanMemoryEntry(row, false)
	if err == nil {
		return &entry, memoryID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, fmt.Errorf("get memory by id: %w", err)
	}

	var path string
	err = r.r.QueryRow(ctx, `
		SELECT path::text
		FROM memory_entries
		WHERE tenant_id = $1::uuid AND id = $2
		LIMIT 1`, tenantID, memoryID).Scan(&path)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, memoryID, fmt.Errorf("memory not found")
	}
	if err != nil {
		return nil, 0, fmt.Errorf("resolve memory path: %w", err)
	}

	resolved, err := r.GetByPath(ctx, tenantID, path, nil, nil)
	if err != nil {
		return nil, memoryID, err
	}
	return resolved, memoryID, nil
}
