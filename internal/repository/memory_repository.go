package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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

// Store appends a new version for path, soft-closing the current row when one exists.
func (r *MemoryRepository) Store(ctx context.Context, req model.StoreRequest, tenantID string) (id int64, version int, supersededID *int64, err error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return 0, 0, nil, fmt.Errorf("path is required")
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

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("store begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	version = 1
	var closedID sql.NullInt64
	var closedVer sql.NullInt32
	closeErr := tx.QueryRow(ctx, `
		UPDATE memory_entries SET valid_to = NOW()
		WHERE tenant_id = $1::uuid AND path = $2::ltree AND valid_to IS NULL
		RETURNING id, version`,
		tenantID, path,
	).Scan(&closedID, &closedVer)
	if closeErr == nil {
		version = int(closedVer.Int32) + 1
		v := closedID.Int64
		supersededID = &v
	} else if closeErr != pgx.ErrNoRows {
		return 0, 0, nil, fmt.Errorf("store close current: %w", closeErr)
	}

	if len(req.Embedding) > 0 {
		q := `
			INSERT INTO memory_entries (tenant_id, path, content, metadata, tags, embedding, embedding_model, version, valid_from, created_at)
			VALUES ($1, $2::ltree, $3, $4, $5, $6, $7, $8, NOW(), NOW())
			RETURNING id`
		err = tx.QueryRow(ctx, q, tenantID, path, req.Content, metadata, tags,
			pgvector.NewVector(req.Embedding), embModel, version).Scan(&id)
	} else {
		q := `
			INSERT INTO memory_entries (tenant_id, path, content, metadata, tags, embedding_model, version, valid_from, created_at)
			VALUES ($1, $2::ltree, $3, $4, $5, $6, $7, NOW(), NOW())
			RETURNING id`
		err = tx.QueryRow(ctx, q, tenantID, path, req.Content, metadata, tags, embModel, version).Scan(&id)
	}
	if err != nil {
		return 0, 0, nil, fmt.Errorf("store insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, nil, fmt.Errorf("store commit: %w", err)
	}
	return id, version, supersededID, nil
}

func (r *MemoryRepository) scanMemoryEntry(rows interface {
	Scan(dest ...any) error
}, includeScore bool) (model.MemoryEntry, error) {
	var e model.MemoryEntry
	var emb *pgvector.Vector
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
	if emb != nil {
		e.Embedding = emb.Slice()
	}
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

// Retrieve returns memories under path_prefix with optional hybrid ranking.
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
	hasText := qText != ""
	hasVec := len(queryEmbedding) > 0
	temporal := temporalClause("$4")

	var q string
	var args []any

	switch {
	case hasVec && hasText:
		vec := pgvector.NewVector(queryEmbedding)
		q = `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at,
			       (
			         0.65 * (1 - (embedding <=> $3::vector))
			         + 0.35 * COALESCE(ts_rank_cd(content_tsv, plainto_tsquery('english', $5)), 0)
			       )::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND ` + temporal + `
			  AND embedding IS NOT NULL
			ORDER BY relevance_score DESC
			LIMIT $6`
		args = []any{tenantID, path, vec, req.AsOf, qText, limit}
	case hasVec:
		vec := pgvector.NewVector(queryEmbedding)
		q = `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at,
			       (1 - (embedding <=> $3::vector))::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND ` + temporal + `
			  AND embedding IS NOT NULL
			ORDER BY embedding <=> $3::vector ASC
			LIMIT $5`
		args = []any{tenantID, path, vec, req.AsOf, limit}
	case hasText:
		q = `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at,
			       ts_rank_cd(content_tsv, plainto_tsquery('english', $3))::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND ` + temporal + `
			  AND content_tsv @@ plainto_tsquery('english', $3)
			ORDER BY relevance_score DESC NULLS LAST, created_at DESC
			LIMIT $5`
		args = []any{tenantID, path, qText, req.AsOf, limit}
	default:
		temporal = temporalClause("$3")
		q = `
			SELECT id, tenant_id, path, content, metadata, tags, embedding, embedding_model,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at,
			       NULL::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND ` + temporal + `
			ORDER BY created_at DESC
			LIMIT $4`
		args = []any{tenantID, path, req.AsOf, limit}
	}

	rows, err := r.db.Query(ctx, q, args...)
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
