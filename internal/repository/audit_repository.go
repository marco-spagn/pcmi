package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
