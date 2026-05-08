package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/pgvector/pgvector-go"
)

type MemoryRepository struct {
	db *pgxpool.Pool
}

func NewMemoryRepository(db *pgxpool.Pool) *MemoryRepository {
	return &MemoryRepository{db: db}
}

func (r *MemoryRepository) Store(ctx context.Context, req model.StoreRequest) (int64, error) {
	query := `
		INSERT INTO memory_entries (
			tenant_id, path, content, metadata, tags, embedding, embedding_model,
			version, valid_from, source_agent_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 1, NOW(), $8)
		RETURNING id`

	var embedding any = nil
	if len(req.Embedding) > 0 {
		embedding = pgvector.NewVector(req.Embedding)
	}

	var sourceAgent any = nil
	if req.SourceAgentID != "" {
		sourceAgent = req.SourceAgentID
	}

	var id int64
	err := r.db.QueryRow(ctx, query,
		req.TenantID, req.Path, req.Content, req.Metadata, req.Tags,
		embedding, req.EmbeddingModel, sourceAgent,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("store failed: %w", err)
	}
	return id, nil
}

func (r *MemoryRepository) Retrieve(ctx context.Context, req model.RetrieveRequest) ([]model.MemoryEntry, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 10
	}

	// Hybrid query: ltree + semantic (quando embedding presente)
	query := `
		SELECT id, tenant_id, path, content, metadata, tags, embedding,
		       embedding_model, version, valid_from, valid_to, source_agent_id, created_at
		FROM memory_entries
		WHERE tenant_id = $1
		  AND path <@ $2::ltree
		  AND (valid_to IS NULL OR valid_to > $3)
		ORDER BY 
			CASE 
				WHEN embedding IS NOT NULL AND $4 IS NOT NULL 
				THEN embedding <=> $4 
				ELSE NULL 
			END,
			created_at DESC
		LIMIT $5`

	// Per ora passiamo un vettore vuoto (placeholder).
	// Nella prossima iterazione passeremo l'embedding della query
	var queryEmbedding pgvector.Vector

	rows, err := r.db.Query(ctx, query,
		req.TenantID,
		req.PathPrefix,
		req.AsOf,
		queryEmbedding,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}
	defer rows.Close()

	var entries []model.MemoryEntry
	for rows.Next() {
		var e model.MemoryEntry
		var emb *pgvector.Vector

		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.Path, &e.Content, &e.Metadata, &e.Tags,
			&emb, &e.EmbeddingModel, &e.Version, &e.ValidFrom, &e.ValidTo,
			&e.SourceAgentID, &e.CreatedAt,
		); err != nil {
			return nil, err
		}

		if emb != nil {
			e.Embedding = emb.Slice()
		}
		entries = append(entries, e)
	}
	return entries, nil
}
