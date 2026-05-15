package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

func (r *MemoryRepository) Store(ctx context.Context, req model.StoreRequest, tenantID string) (int64, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return 0, fmt.Errorf("path is required")
	}
	embModel := req.EmbeddingModel
	if embModel == "" {
		embModel = "unspecified"
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	var id int64
	var err error
	if len(req.Embedding) > 0 {
		q := `
			INSERT INTO memory_entries (tenant_id, path, content, metadata, tags, embedding, embedding_model, version, valid_from, created_at)
			VALUES ($1, $2::ltree, $3, $4, $5, $6, $7, 1, NOW(), NOW())
			RETURNING id`
		err = r.db.QueryRow(ctx, q, tenantID, path, req.Content, metadata, tags,
			pgvector.NewVector(req.Embedding), embModel).Scan(&id)
	} else {
		q := `
			INSERT INTO memory_entries (tenant_id, path, content, metadata, tags, embedding_model, version, valid_from, created_at)
			VALUES ($1, $2::ltree, $3, $4, $5, $6, 1, NOW(), NOW())
			RETURNING id`
		err = r.db.QueryRow(ctx, q, tenantID, path, req.Content, metadata, tags, embModel).Scan(&id)
	}
	if err != nil {
		return 0, fmt.Errorf("store failed: %w", err)
	}
	return id, nil
}

func (r *MemoryRepository) scanMemoryEntry(rows interface {
	Scan(dest ...any) error
}, includeScore bool) (model.MemoryEntry, error) {
	var e model.MemoryEntry
	var emb pgvector.Vector
	var validTo sql.NullTime
	var agentID sql.NullString
	var eventID sql.NullString
	var score sql.NullFloat64

	dest := []any{
		&e.ID, &e.TenantID, &e.Path, &e.Content, &e.Metadata, &e.Tags,
		&emb, &e.EmbeddingModel, &e.Version, &e.ValidFrom, &validTo,
		&agentID, &eventID, &e.CreatedAt,
	}
	if includeScore {
		dest = append(dest, &score)
	}

	if err := rows.Scan(dest...); err != nil {
		return e, err
	}
	e.Embedding = emb.Slice()
	if validTo.Valid {
		t := validTo.Time
		e.ValidTo = &t
	}
	if agentID.Valid {
		s := agentID.String
		e.SourceAgentID = &s
	}
	if eventID.Valid {
		s := eventID.String
		e.SourceEventID = &s
	}
	if includeScore && score.Valid {
		e.RelevanceScore = score.Float64
	}
	return e, nil
}

// Retrieve returns memories under path_prefix, optionally ranked by semantic similarity to queryEmbedding.
func (r *MemoryRepository) Retrieve(ctx context.Context, req model.RetrieveRequest, tenantID string, queryEmbedding []float32) ([]model.MemoryEntry, error) {
	path := strings.TrimSpace(req.PathPrefix)
	if path == "" {
		path = "root"
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}

	qText := strings.TrimSpace(req.Query)

	var err error
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Err() error
		Close()
	}

	if len(queryEmbedding) > 0 {
		vec := pgvector.NewVector(queryEmbedding)
		q := `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at,
			       (1 - (embedding <=> $3::vector))::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND valid_to IS NULL
			  AND embedding IS NOT NULL
			  AND ($4::timestamptz IS NULL OR (valid_from <= $4 AND (valid_to IS NULL OR valid_to > $4)))
			ORDER BY embedding <=> $3::vector ASC
			LIMIT $5`
		rows, err = r.db.Query(ctx, q, tenantID, path, vec, req.AsOf, limit)
	} else {
		q := `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at,
			       NULL::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND valid_to IS NULL
			  AND ($3::text = '' OR position(lower($3) in lower(content)) > 0)
			  AND ($4::timestamptz IS NULL OR (valid_from <= $4 AND (valid_to IS NULL OR valid_to > $4)))
			ORDER BY created_at DESC
			LIMIT $5`
		rows, err = r.db.Query(ctx, q, tenantID, path, qText, req.AsOf, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("retrieve failed: %w", err)
	}
	defer rows.Close()

	var entries []model.MemoryEntry
	for rows.Next() {
		e, scanErr := r.scanMemoryEntry(rows, true)
		if scanErr != nil {
			return nil, fmt.Errorf("retrieve scan: %w", scanErr)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
