package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UsageRow is one llm_usage_daily bucket.
type UsageRow struct {
	Day          string
	Operation    string
	Provider     string
	Model        string
	Requests     int64
	InputTokens  int64
	OutputTokens int64
}

// UsageRepository reads llm_usage_daily (migration 029).
type UsageRepository struct {
	db *pgxpool.Pool
}

func NewUsageRepository(db *pgxpool.Pool) *UsageRepository {
	return &UsageRepository{db: db}
}

// Daily returns the tenant's buckets with from <= day <= to (UTC dates).
func (r *UsageRepository) Daily(ctx context.Context, tenantID string, from, to time.Time) ([]UsageRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT to_char(day, 'YYYY-MM-DD'), operation, provider, model, requests, input_tokens, output_tokens
		FROM llm_usage_daily
		WHERE tenant_id = $1::uuid AND day BETWEEN $2::date AND $3::date
		ORDER BY day, operation, provider, model`,
		tenantID, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("usage daily: %w", err)
	}
	defer rows.Close()
	var out []UsageRow
	for rows.Next() {
		var u UsageRow
		if err := rows.Scan(&u.Day, &u.Operation, &u.Provider, &u.Model, &u.Requests, &u.InputTokens, &u.OutputTokens); err != nil {
			return nil, fmt.Errorf("usage scan: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
