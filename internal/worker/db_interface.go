package worker

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// workerDB is satisfied by *pgxpool.Pool and by pgxmock pools in tests.
// It includes Begin so that it also satisfies repository.memoryWriteDB/memoryReadDB,
// allowing ConsolidationWorker to pass its db directly to NewMemoryRepositoryFromDB.
type workerDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}
