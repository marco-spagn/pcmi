package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/auditchain"
	"github.com/marco-spagn/pcmi/internal/model"
)

type AuditRepository struct {
	db *pgxpool.Pool
}

func NewAuditRepository(db *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{db: db}
}

// Count returns the number of audit rows for a tenant (optional since filter).
func (r *AuditRepository) Count(ctx context.Context, tenantID string, since *time.Time) (int, error) {
	countQ := `SELECT COUNT(*) FROM audit_log WHERE tenant_id = $1::uuid`
	args := []any{tenantID}
	if since != nil {
		countQ += ` AND created_at >= $2`
		args = append(args, *since)
	}
	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("audit count: %w", err)
	}
	return total, nil
}

func (r *AuditRepository) List(
	ctx context.Context,
	tenantID string,
	page model.PageRequest,
	since *time.Time,
) ([]model.AuditEntry, model.PageResponse, error) {
	limit := page.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	sortKey := model.SortKeyCreatedAtIDDesc
	if !page.Cursor.IsZero() && page.Cursor.SortKey != "" {
		sortKey = page.Cursor.SortKey
	}

	listQ := `
		SELECT id, tenant_id::text, api_key_id::text, event_type, path, method,
		       status_code, ip_address::text, user_agent, created_at
		FROM audit_log
		WHERE tenant_id = $1::uuid`
	listArgs := []any{tenantID}
	argN := 2

	if since != nil {
		listQ += fmt.Sprintf(` AND created_at >= $%d`, argN)
		listArgs = append(listArgs, *since)
		argN++
	}

	clause, clauseArgs, err := KeysetCreatedAtIDClause(page.Cursor, sortKey, argN)
	if err != nil {
		return nil, model.PageResponse{}, err
	}
	listQ += clause
	listArgs = append(listArgs, clauseArgs...)
	argN += len(clauseArgs)

	listQ += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, argN)
	listArgs = append(listArgs, FetchLimit(limit))

	rows, err := r.db.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, model.PageResponse{}, fmt.Errorf("audit list: %w", err)
	}
	defer rows.Close()

	var entries []model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		var apiKeyID, ip, ua *string
		if err := rows.Scan(
			&e.ID, &e.TenantID, &apiKeyID, &e.EventType, &e.Path, &e.Method,
			&e.StatusCode, &ip, &ua, &e.CreatedAt,
		); err != nil {
			return nil, model.PageResponse{}, fmt.Errorf("audit scan: %w", err)
		}
		e.APIKeyID = apiKeyID
		e.IPAddress = ip
		e.UserAgent = ua
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, model.PageResponse{}, err
	}
	if entries == nil {
		entries = []model.AuditEntry{}
	}

	trimmed, pageResp, err := model.FinishInt64Page(entries, limit, sortKey,
		func(e model.AuditEntry) int64 { return e.ID },
		func(e model.AuditEntry) time.Time { return e.CreatedAt },
	)
	return trimmed, pageResp, err
}

// auditChainColumns renders every chained column in the exact canonical text
// form that audit_log_canonical (migration 027) hashes, so auditchain.Canonical
// can recompute the hash in Go.
const auditChainColumns = `
	id,
	tenant_id::text,
	chain_seq,
	COALESCE(api_key_id::text, ''),
	COALESCE(event_type, ''),
	COALESCE(path, ''),
	COALESCE(method, ''),
	status_code,
	COALESCE(request_body::text, ''),
	COALESCE(response_body::text, ''),
	COALESCE(host(ip_address), ''),
	COALESCE(user_agent, ''),
	COALESCE(to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'), ''),
	prev_hash,
	row_hash`

// ChainBatch returns up to limit chained audit rows of a tenant with
// chain_seq > afterSeq (and <= toSeq when toSeq > 0), ascending.
func (r *AuditRepository) ChainBatch(ctx context.Context, tenantID string, afterSeq, toSeq int64, limit int) ([]auditchain.Record, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+auditChainColumns+`
		FROM audit_log
		WHERE tenant_id = $1::uuid
		  AND chain_seq > $2
		  AND ($3::bigint = 0 OR chain_seq <= $3)
		ORDER BY chain_seq
		LIMIT $4`, tenantID, afterSeq, toSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("audit chain batch: %w", err)
	}
	defer rows.Close()
	var out []auditchain.Record
	for rows.Next() {
		var rec auditchain.Record
		if err := rows.Scan(
			&rec.ID, &rec.TenantID, &rec.ChainSeq, &rec.APIKeyID, &rec.EventType,
			&rec.Path, &rec.Method, &rec.StatusCode, &rec.RequestBody, &rec.ResponseBody,
			&rec.IPAddress, &rec.UserAgent, &rec.CreatedAt, &rec.PrevHash, &rec.RowHash,
		); err != nil {
			return nil, fmt.Errorf("audit chain scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ChainHead returns the tenant's latest chain_seq and row_hash (0, "" when empty).
func (r *AuditRepository) ChainHead(ctx context.Context, tenantID string) (int64, string, error) {
	var (
		seq  int64
		head string
	)
	err := r.db.QueryRow(ctx, `
		SELECT chain_seq, row_hash FROM audit_log
		WHERE tenant_id = $1::uuid AND chain_seq IS NOT NULL
		ORDER BY chain_seq DESC LIMIT 1`, tenantID).Scan(&seq, &head)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("audit chain head: %w", err)
	}
	return seq, head, nil
}
