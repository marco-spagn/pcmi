package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/marco-spagn/pcmi/internal/model"
)

// GetByPath returns the current memory at path, or a specific version / point-in-time slice.
func (r *MemoryRepository) GetByPath(
	ctx context.Context,
	tenantID, path string,
	version *int,
	asOf *time.Time,
) (*model.MemoryEntry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if version != nil {
		return r.GetHistoricalVersion(ctx, tenantID, path, version, nil)
	}
	if asOf != nil {
		return r.GetHistoricalVersion(ctx, tenantID, path, nil, asOf)
	}

	q := `
		SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model, embedding_space,
		       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at, content_encrypted
		FROM memory_entries
		WHERE tenant_id = $1::uuid AND path = $2::ltree AND valid_to IS NULL
		LIMIT 1`

	row := r.r.QueryRow(ctx, q, tenantID, path)
	e, err := r.scanMemoryEntry(row, false)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("memory not found for path %s", path)
	}
	if err != nil {
		return nil, fmt.Errorf("get by path: %w", err)
	}
	return &e, nil
}
