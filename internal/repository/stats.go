package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
)

// statsQueryRowDB is implemented by *pgxpool.Pool and pgxmock pools.
type statsQueryRowDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type StatsRepository struct {
	r statsQueryRowDB
}

func NewStatsRepository(writePool, readPool *pgxpool.Pool) *StatsRepository {
	if readPool == nil {
		readPool = writePool
	}
	return &StatsRepository{r: readPool}
}

func (r *StatsRepository) TenantStats(ctx context.Context, tenantID string) (*model.StatsResponse, error) {
	var s model.StatsResponse
	err := r.r.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM memory_entries WHERE tenant_id = $1::uuid AND valid_to IS NULL),
		  (SELECT COUNT(*) FROM memory_entries WHERE tenant_id = $1::uuid AND valid_to IS NOT NULL),
		  (SELECT COUNT(*) FROM distilled_knowledge WHERE tenant_id = $1::uuid),
		  (SELECT COUNT(*) FROM memory_links WHERE tenant_id = $1::uuid),
		  (SELECT COUNT(*) FROM events WHERE tenant_id = $1::uuid),
		  (SELECT COUNT(*) FROM memory_entries
		     WHERE tenant_id = $1::uuid AND valid_to IS NULL
		       AND expires_at IS NOT NULL AND expires_at <= NOW() + interval '24 hours')`,
		tenantID,
	).Scan(&s.ActiveMemories, &s.SupersededMemories, &s.DistilledCount, &s.LinksCount, &s.EventsCount, &s.ExpiringSoon)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
