package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/marco-spagn/pcmi/internal/model"
)

// GetHistoricalVersion returns the memory row active at as_of or the exact version.
func (r *MemoryRepository) GetHistoricalVersion(
	ctx context.Context,
	tenantID, path string,
	version *int,
	asOf *time.Time,
) (*model.MemoryEntry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if version == nil && asOf == nil {
		return nil, fmt.Errorf("version or as_of is required")
	}
	if version != nil && asOf != nil {
		return nil, fmt.Errorf("provide only one of version or as_of")
	}

	var q string
	var args []any

	if version != nil {
		q = `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model, embedding_space,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at, content_encrypted
			FROM memory_entries
			WHERE tenant_id = $1::uuid AND path = $2::ltree AND version = $3
			LIMIT 1`
		args = []any{tenantID, path, *version}
	} else {
		temporal := temporalClause("$3")
		q = `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model, embedding_space,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at, content_encrypted
			FROM memory_entries
			WHERE tenant_id = $1::uuid AND path = $2::ltree AND ` + temporal + `
			ORDER BY version DESC
			LIMIT 1`
		args = []any{tenantID, path, asOf}
	}

	row := r.db.QueryRow(ctx, q, args...)
	e, err := r.scanMemoryEntry(row, false)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("no historical version found for path %s", path)
	}
	if err != nil {
		return nil, fmt.Errorf("get historical version: %w", err)
	}
	return &e, nil
}
