package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const idempotencyTTL = 24 * time.Hour

// IdempotencyDB is implemented by *pgxpool.Pool and pgxmock pools.
type IdempotencyDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// IdempotencyRepository persists cached store responses per tenant + key.
type IdempotencyRepository struct {
	db IdempotencyDB
}

func NewIdempotencyRepository(db IdempotencyDB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

// Get returns a non-expired cached response body, or nil if missing/expired.
func (r *IdempotencyRepository) Get(ctx context.Context, tenantID, key string) (json.RawMessage, bool, error) {
	var raw []byte
	err := r.db.QueryRow(ctx, `
		SELECT response_json
		FROM idempotency_cache
		WHERE tenant_id = $1::uuid
		  AND idempotency_key = $2
		  AND expires_at > NOW()
	`, tenantID, key).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("idempotency get: %w", err)
	}
	return json.RawMessage(raw), true, nil
}

// Put stores the response JSON with a 24h expiry (upsert).
func (r *IdempotencyRepository) Put(ctx context.Context, tenantID, key string, response json.RawMessage) error {
	expires := time.Now().Add(idempotencyTTL)
	_, err := r.db.Exec(ctx, `
		INSERT INTO idempotency_cache (idempotency_key, tenant_id, response_json, expires_at)
		VALUES ($1, $2::uuid, $3::jsonb, $4)
		ON CONFLICT (tenant_id, idempotency_key) DO UPDATE
		SET response_json = EXCLUDED.response_json,
		    expires_at = EXCLUDED.expires_at,
		    created_at = NOW()
	`, key, tenantID, response, expires)
	if err != nil {
		return fmt.Errorf("idempotency put: %w", err)
	}
	return nil
}

// DeleteExpired removes rows past expires_at; returns rows deleted.
func (r *IdempotencyRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM idempotency_cache WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("idempotency delete expired: %w", err)
	}
	return tag.RowsAffected(), nil
}
