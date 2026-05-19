package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pcmicrypto "github.com/marco-spagn/pcmi/internal/crypto"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/pgvector/pgvector-go"
)

type MemoryRepository struct {
	w *pgxpool.Pool // primary: transactions, inserts, strong reads (rollback / historical)
	r *pgxpool.Pool // read pool: replica when configured, else same as w
}

// NewMemoryRepository routes writes to writePool and SELECT-heavy paths to readPool.
// If readPool is nil, readPool defaults to writePool.
func NewMemoryRepository(writePool, readPool *pgxpool.Pool) *MemoryRepository {
	if readPool == nil {
		readPool = writePool
	}
	return &MemoryRepository{w: writePool, r: readPool}
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
	embSpace := strings.TrimSpace(req.EmbeddingSpace)
	if embSpace == "" {
		embSpace = "default"
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	content := req.Content
	contentEncrypted := false
	if pcmicrypto.ShouldEncrypt(req.EncryptContent, metadata) {
		enc, encErr := pcmicrypto.EncryptContent(content)
		if encErr != nil {
			return 0, 0, nil, fmt.Errorf("encrypt content: %w", encErr)
		}
		content = enc
		contentEncrypted = true
	}

	tx, err := r.w.Begin(ctx)
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

	var agentID *string
	if a := strings.TrimSpace(req.SourceAgentID); a != "" {
		agentID = &a
	}

	if len(req.Embedding) > 0 {
		q := `
			INSERT INTO memory_entries (tenant_id, path, content, metadata, tags, embedding, embedding_model, embedding_space, version, valid_from, source_agent_id, created_at, content_encrypted, expires_at)
			VALUES ($1, $2::ltree, $3, $4, $5, $6, $7, $8, $9, NOW(), $10::uuid, NOW(), $11, $12)
			RETURNING id`
		err = tx.QueryRow(ctx, q, tenantID, path, content, metadata, tags,
			pgvector.NewVector(req.Embedding), embModel, embSpace, version, agentID, contentEncrypted, req.ExpiresAt).Scan(&id)
	} else {
		q := `
			INSERT INTO memory_entries (tenant_id, path, content, metadata, tags, embedding_model, embedding_space, version, valid_from, source_agent_id, created_at, content_encrypted, expires_at)
			VALUES ($1, $2::ltree, $3, $4, $5, $6, $7, $8, NOW(), $9::uuid, NOW(), $10, $11)
			RETURNING id`
		err = tx.QueryRow(ctx, q, tenantID, path, content, metadata, tags, embModel, embSpace, version, agentID, contentEncrypted, req.ExpiresAt).Scan(&id)
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
		&emb, &e.EmbeddingModel, &e.EmbeddingSpace, &e.Version, &e.ValidFrom, &validTo,
		&agentID, &eventID, &e.CreatedAt, &e.ContentEncrypted,
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
	if e.ContentEncrypted {
		plain, decErr := pcmicrypto.DecryptContent(e.Content)
		if decErr != nil {
			return e, fmt.Errorf("decrypt content id=%d: %w", e.ID, decErr)
		}
		e.Content = plain
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

	agentFilter := strings.TrimSpace(req.SourceAgentID)
	spaceFilter := strings.TrimSpace(req.EmbeddingSpace)
	tagList := req.Tags
	if tagList == nil {
		tagList = []string{}
	}
	tagMatch := strings.TrimSpace(req.TagsMatch)
	if tagMatch == "" {
		tagMatch = "any"
	}

	selectCols := `id, tenant_id, path, content, metadata, tags, embedding, embedding_model, embedding_space,
			       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at, content_encrypted`

	var q string
	var args []any

	if strings.TrimSpace(req.Cursor) != "" && (hasText || hasVec) {
		return nil, fmt.Errorf("cursor pagination requires empty query and no embedding search")
	}

	switch {
	case hasVec && hasText:
		vec := pgvector.NewVector(queryEmbedding)
		q = `
			SELECT ` + selectCols + `,
			       (
			         0.55 * (1 - (embedding <=> $3::vector))
			         + 0.45 * pcmi_bm25_rank(content_tsv, websearch_to_tsquery('english', $5))
			       )::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND ` + temporalClause("$4") + `
			  AND ` + scopeFilters("6", "7") + `
			  AND ` + tagFilters("8", "9") + `
			  AND embedding IS NOT NULL
			ORDER BY relevance_score DESC
			LIMIT $10`
		args = []any{tenantID, path, vec, req.AsOf, qText, agentFilter, spaceFilter, tagList, tagMatch, limit}
	case hasVec:
		vec := pgvector.NewVector(queryEmbedding)
		q = `
			SELECT ` + selectCols + `,
			       (1 - (embedding <=> $3::vector))::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND ` + temporalClause("$4") + `
			  AND ` + scopeFilters("5", "6") + `
			  AND ` + tagFilters("7", "8") + `
			  AND embedding IS NOT NULL
			ORDER BY embedding <=> $3::vector ASC
			LIMIT $9`
		args = []any{tenantID, path, vec, req.AsOf, agentFilter, spaceFilter, tagList, tagMatch, limit}
	case hasText:
		q = `
			SELECT ` + selectCols + `,
			       pcmi_bm25_rank(content_tsv, websearch_to_tsquery('english', $3))::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND ` + temporalClause("$4") + `
			  AND ` + scopeFilters("5", "6") + `
			  AND ` + tagFilters("7", "8") + `
			  AND content_tsv @@ websearch_to_tsquery('english', $3)
			ORDER BY relevance_score DESC NULLS LAST, created_at DESC
			LIMIT $9`
		args = []any{tenantID, path, qText, req.AsOf, agentFilter, spaceFilter, tagList, tagMatch, limit}
	default:
		cur, curErr := model.DecodeCursor(req.Cursor)
		if curErr != nil {
			return nil, fmt.Errorf("invalid cursor: %w", curErr)
		}
		if !cur.IsZero() && cur.SortKey != "" && cur.SortKey != model.SortKeyCreatedAtIDDesc {
			return nil, fmt.Errorf("cursor sort key mismatch: got %q", cur.SortKey)
		}
		fetchLimit := limit + 1
		args = []any{tenantID, path, req.AsOf, agentFilter, spaceFilter, tagList, tagMatch}
		cursorClause := ""
		if !cur.IsZero() {
			cursorClause = fmt.Sprintf(" AND (created_at, id) < ($%d::timestamptz, $%d::bigint)", len(args)+1, len(args)+2)
			args = append(args, cur.LastTimestamp, cur.LastID)
		}
		limitPos := len(args) + 1
		args = append(args, fetchLimit)
		q = fmt.Sprintf(`
			SELECT %s,
			       NULL::float8 AS relevance_score
			FROM memory_entries
			WHERE tenant_id = $1::uuid
			  AND path <@ $2::ltree
			  AND %s
			  AND %s
			  AND %s%s
			ORDER BY created_at DESC, id DESC
			LIMIT $%d`, selectCols, temporalClause("$3"), scopeFilters("4", "5"), tagFilters("6", "7"), cursorClause, limitPos)
	}

	rows, err := r.r.Query(ctx, q, args...)
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

// CompactPathHistory deletes older superseded rows for a path, keeping the newest keepSuperseded closed versions.
func (r *MemoryRepository) CompactPathHistory(ctx context.Context, tenantID, path string, keepSuperseded int) (int, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, fmt.Errorf("path is required")
	}
	if keepSuperseded < 1 {
		keepSuperseded = 20
	}
	if keepSuperseded > 500 {
		keepSuperseded = 500
	}
	var n int
	err := r.w.QueryRow(ctx, `SELECT compact_memory_path_history($1::uuid, $2::ltree, $3)`,
		tenantID, path, keepSuperseded).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("compact path history: %w", err)
	}
	return n, nil
}
