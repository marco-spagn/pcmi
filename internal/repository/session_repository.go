package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
)

const (
	sessionMetadataKey   = "session_id"
	sessionScopeKey      = "memory_scope"
	sessionScopeWorking  = "working"
	sessionScopeLongTerm = "long_term"
)

// sessionWriteDB is implemented by *pgxpool.Pool and pgxmock pools.
type sessionWriteDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// sessionReadDB is implemented by *pgxpool.Pool and pgxmock pools.
type sessionReadDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// SessionRepository persists agent_sessions and session-scoped memory queries.
type SessionRepository struct {
	w sessionWriteDB
	r sessionReadDB
}

func NewSessionRepository(writePool, readPool *pgxpool.Pool) *SessionRepository {
	if readPool == nil {
		readPool = writePool
	}
	return NewSessionRepositoryFromDB(writePool, readPool)
}

// NewSessionRepositoryFromDB wires session persistence (pgxmock in unit tests).
func NewSessionRepositoryFromDB(w sessionWriteDB, r sessionReadDB) *SessionRepository {
	if r == nil {
		r, _ = w.(sessionReadDB)
	}
	return &SessionRepository{w: w, r: r}
}

func (r *SessionRepository) Create(ctx context.Context, tenantID string, req model.CreateSessionRequest) (*model.AgentSession, error) {
	meta := req.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	var agentID *string
	if a := strings.TrimSpace(req.AgentID); a != "" {
		agentID = &a
	}
	var sess model.AgentSession
	err = r.w.QueryRow(ctx, `
		INSERT INTO agent_sessions (tenant_id, agent_id, metadata)
		VALUES ($1::uuid, $2::uuid, $3::jsonb)
		RETURNING id::text, tenant_id::text, agent_id::text, metadata, started_at, ended_at`,
		tenantID, agentID, metaJSON,
	).Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &metaJSON, &sess.StartedAt, &sess.EndedAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	_ = json.Unmarshal(metaJSON, &sess.Metadata)
	sess.Status = sessionStatus(sess.EndedAt)
	return &sess, nil
}

func (r *SessionRepository) Get(ctx context.Context, tenantID, sessionID string) (*model.AgentSession, error) {
	var sess model.AgentSession
	var metaJSON []byte
	err := r.r.QueryRow(ctx, `
		SELECT id::text, tenant_id::text, agent_id::text, metadata, started_at, ended_at
		FROM agent_sessions
		WHERE id = $1::uuid AND tenant_id = $2::uuid`,
		sessionID, tenantID,
	).Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &metaJSON, &sess.StartedAt, &sess.EndedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	_ = json.Unmarshal(metaJSON, &sess.Metadata)
	sess.Status = sessionStatus(sess.EndedAt)
	return &sess, nil
}

func (r *SessionRepository) End(ctx context.Context, tenantID, sessionID string) (*model.AgentSession, error) {
	var sess model.AgentSession
	var metaJSON []byte
	err := r.w.QueryRow(ctx, `
		UPDATE agent_sessions
		SET ended_at = COALESCE(ended_at, NOW())
		WHERE id = $1::uuid AND tenant_id = $2::uuid
		RETURNING id::text, tenant_id::text, agent_id::text, metadata, started_at, ended_at`,
		sessionID, tenantID,
	).Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &metaJSON, &sess.StartedAt, &sess.EndedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("end session: %w", err)
	}
	_ = json.Unmarshal(metaJSON, &sess.Metadata)
	sess.Status = sessionStatus(sess.EndedAt)
	return &sess, nil
}

func sessionStatus(endedAt *time.Time) string {
	if endedAt != nil {
		return "ended"
	}
	return "active"
}

// ListSessionMemories returns working-memory rows for the session; when includeLongTerm is true,
// appends current long-term rows under pathPrefix that are not session-scoped (session entries first).
func (r *SessionRepository) ListSessionMemories(ctx context.Context, tenantID, sessionID, pathPrefix string, limit int, includeLongTerm bool) ([]model.MemoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.r.Query(ctx, `
		SELECT id, tenant_id::text, path::text, content, metadata, tags, embedding_model,
		       COALESCE(embedding_space, 'default'), version, valid_from, valid_to,
		       source_agent_id::text, created_at,
		       COALESCE(importance, 0.5), COALESCE(access_count, 0), last_accessed_at
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND valid_to IS NULL
		  AND metadata->>'session_id' = $2
		ORDER BY created_at DESC
		LIMIT $3`,
		tenantID, sessionID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list session memories: %w", err)
	}
	defer rows.Close()

	entries, err := scanMemoryRows(rows, false)
	if err != nil {
		return nil, err
	}
	if !includeLongTerm {
		return entries, nil
	}
	remaining := limit - len(entries)
	if remaining <= 0 {
		return entries, nil
	}
	prefix := strings.TrimSpace(pathPrefix)
	if prefix == "" {
		prefix = "root"
	}
	ltRows, err := r.r.Query(ctx, `
		SELECT id, tenant_id::text, path::text, content, metadata, tags, embedding_model,
		       COALESCE(embedding_space, 'default'), version, valid_from, valid_to,
		       source_agent_id::text, created_at,
		       COALESCE(importance, 0.5), COALESCE(access_count, 0), last_accessed_at
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND valid_to IS NULL
		  AND path <@ $2::ltree
		  AND (metadata->>'session_id' IS NULL OR metadata->>'session_id' = '')
		ORDER BY created_at DESC
		LIMIT $3`,
		tenantID, prefix, remaining,
	)
	if err != nil {
		return nil, fmt.Errorf("list long-term memories: %w", err)
	}
	defer ltRows.Close()
	ltEntries, err := scanMemoryRows(ltRows, false)
	if err != nil {
		return nil, err
	}
	return append(entries, ltEntries...), nil
}

func scanMemoryRows(rows pgx.Rows, includeScore bool) ([]model.MemoryEntry, error) {
	var out []model.MemoryEntry
	for rows.Next() {
		var e model.MemoryEntry
		var metaJSON []byte
		var tags []string
		var validTo *time.Time
		var agentID *string
		var lastAccessed *time.Time
		dest := []any{
			&e.ID, &e.TenantID, &e.Path, &e.Content, &metaJSON, &tags,
			&e.EmbeddingModel, &e.EmbeddingSpace, &e.Version, &e.ValidFrom, &validTo,
			&agentID, &e.CreatedAt, &e.Importance, &e.AccessCount, &lastAccessed,
		}
		if includeScore {
			dest = append(dest, &e.RelevanceScore)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		_ = json.Unmarshal(metaJSON, &e.Metadata)
		e.Tags = tags
		e.ValidTo = validTo
		e.SourceAgentID = agentID
		e.LastAccessedAt = lastAccessed
		out = append(out, e)
	}
	return out, rows.Err()
}

// Promote re-paths a session's working-memory rows to long-term paths under
// targetPrefix, clearing session-scope metadata. It returns how many were
// promoted and how many were skipped because the target path already holds a
// current memory (see below).
func (r *SessionRepository) Promote(ctx context.Context, tenantID, sessionID, targetPrefix string) (promoted int, skipped int, err error) {
	targetPrefix = strings.TrimSpace(targetPrefix)
	if targetPrefix == "" {
		targetPrefix = "root"
	}
	tx, err := r.w.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("promote begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, path::text, metadata
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND valid_to IS NULL
		  AND metadata->>'session_id' = $2
		FOR UPDATE`,
		tenantID, sessionID,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("promote select: %w", err)
	}
	defer rows.Close()

	type row struct {
		id       int64
		path     string
		metadata map[string]any
	}
	var batch []row
	for rows.Next() {
		var id int64
		var path string
		var metaJSON []byte
		if err := rows.Scan(&id, &path, &metaJSON); err != nil {
			return 0, 0, fmt.Errorf("promote scan: %w", err)
		}
		meta := map[string]any{}
		_ = json.Unmarshal(metaJSON, &meta)
		batch = append(batch, row{id: id, path: path, metadata: meta})
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, item := range batch {
		newPath := promotePath(item.path, sessionID, targetPrefix)

		// A promote is an in-place re-path of the working-memory row. If a
		// current (valid_to IS NULL) memory already occupies the target path,
		// the UPDATE would violate uq_memory_entries_open_version (migration
		// 020) and abort the whole transaction. Skip such items — including two
		// session rows that map to the same target — and report them, rather
		// than failing the entire promotion or overwriting existing long-term
		// data.
		var occupied bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM memory_entries
				WHERE tenant_id = $1::uuid AND path = $2::ltree
				  AND valid_to IS NULL AND id <> $3
			)`, tenantID, newPath, item.id).Scan(&occupied); err != nil {
			return 0, 0, fmt.Errorf("promote collision check id=%d: %w", item.id, err)
		}
		if occupied {
			skipped++
			continue
		}

		meta := cloneMetadata(item.metadata)
		delete(meta, sessionMetadataKey)
		meta[sessionScopeKey] = sessionScopeLongTerm
		meta["promoted_from_session"] = sessionID
		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return 0, 0, err
		}
		if _, err = tx.Exec(ctx, `
			UPDATE memory_entries
			SET path = $2::ltree, metadata = $3::jsonb
			WHERE id = $1`,
			item.id, newPath, metaJSON,
		); err != nil {
			return 0, 0, fmt.Errorf("promote update id=%d: %w", item.id, err)
		}
		promoted++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("promote commit: %w", err)
	}
	return promoted, skipped, nil
}

func promotePath(currentPath, sessionID, targetPrefix string) string {
	prefix := "sessions." + strings.ReplaceAll(sessionID, "-", "_") + "."
	altPrefix := "sessions." + sessionID + "."
	suffix := ""
	switch {
	case strings.HasPrefix(currentPath, prefix):
		suffix = strings.TrimPrefix(currentPath, prefix)
	case strings.HasPrefix(currentPath, altPrefix):
		suffix = strings.TrimPrefix(currentPath, altPrefix)
	default:
		suffix = strings.TrimPrefix(currentPath, "sessions.")
	}
	suffix = strings.Trim(suffix, ".")
	if suffix == "" {
		return targetPrefix
	}
	return targetPrefix + "." + suffix
}

func cloneMetadata(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// SessionScopedStoreRequest builds a StoreRequest with session metadata and path prefix.
func SessionScopedStoreRequest(sessionID string, req model.SessionStoreMemoryRequest) model.StoreRequest {
	path := strings.TrimSpace(req.Path)
	prefix := "sessions." + strings.ReplaceAll(sessionID, "-", "_")
	if path == "" {
		path = prefix + ".note"
	} else if !strings.HasPrefix(path, prefix+".") && !strings.HasPrefix(path, "sessions.") {
		path = prefix + "." + strings.TrimPrefix(path, ".")
	}
	meta := req.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	meta[sessionMetadataKey] = sessionID
	meta[sessionScopeKey] = sessionScopeWorking
	store := model.StoreRequest{
		Path:           path,
		Content:        req.Content,
		Metadata:       meta,
		Tags:           req.Tags,
		EmbeddingModel: req.EmbeddingModel,
		SourceAgentID:  req.SourceAgentID,
		Importance:     req.Importance,
	}
	if store.EmbeddingModel == "" {
		store.EmbeddingModel = "unspecified"
	}
	return store
}
