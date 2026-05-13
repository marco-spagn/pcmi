package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/model"
)

type MemoryRepository struct {
	db *pgxpool.Pool
}

func NewMemoryRepository(db *pgxpool.Pool) *MemoryRepository {
	return &MemoryRepository{db: db}
}

func (r *MemoryRepository) Store(ctx context.Context, req model.StoreRequest, tenantID string) (int64, error) {
	query := `
		INSERT INTO memory_entries (tenant_id, path, content, metadata, tags, embedding_model, version, valid_from, created_at)
		VALUES ($1, $2::ltree, $3, $4, $5, $6, 1, NOW(), NOW())
		RETURNING id`

	var id int64
	err := r.db.QueryRow(ctx, query, tenantID, req.Path, req.Content, req.Metadata, req.Tags, req.EmbeddingModel).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store failed: %w", err)
	}
	return id, nil
}

func (r *MemoryRepository) Retrieve(ctx context.Context, req model.RetrieveRequest, tenantID string) ([]model.MemoryEntry, error) {
	query := `
		SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model,
		       version, valid_from, valid_to, source_agent_id, source_event_id, created_at
		FROM memory_entries
		WHERE tenant_id = $1 AND path <@ $2::ltree
		ORDER BY created_at DESC LIMIT $3`

	rows, err := r.db.Query(ctx, query, tenantID, req.PathPrefix, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}
	defer rows.Close()

	var entries []model.MemoryEntry
	for rows.Next() {
		var e model.MemoryEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Path, &e.Content, &e.Metadata, &e.Tags,
			&e.Embedding, &e.EmbeddingModel, &e.Version, &e.ValidFrom, &e.ValidTo,
			&e.SourceAgentID, &e.SourceEventID, &e.CreatedAt); err == nil {
			entries = append(entries, e)
		}
	}
	return entries, nil
}
