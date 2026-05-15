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

func (r *AuditRepository) List(
	ctx context.Context,
	tenantID string,
	limit, offset int,
	since *time.Time,
) ([]model.AuditEntry, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	countQ := `SELECT COUNT(*) FROM audit_log WHERE tenant_id = $1::uuid`
	countArgs := []any{tenantID}
	listQ := `
		SELECT id, tenant_id::text, api_key_id::text, event_type, path, method,
		       status_code, ip_address::text, user_agent, created_at
		FROM audit_log
		WHERE tenant_id = $1::uuid`
	listArgs := []any{tenantID}

	if since != nil {
		countQ += ` AND created_at >= $2`
		listQ += ` AND created_at >= $2`
		countArgs = append(countArgs, *since)
		listArgs = append(listArgs, *since)
	}

	var total int
	if err := r.db.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit count: %w", err)
	}

	listQ += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(listArgs)+1) +
		` OFFSET $` + fmt.Sprint(len(listArgs)+2)
	listArgs = append(listArgs, limit, offset)

	rows, err := r.db.Query(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit list: %w", err)
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
			return nil, 0, fmt.Errorf("audit scan: %w", err)
		}
		e.APIKeyID = apiKeyID
		e.IPAddress = ip
		e.UserAgent = ua
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []model.AuditEntry{}
	}
	return entries, total, rows.Err()
}
