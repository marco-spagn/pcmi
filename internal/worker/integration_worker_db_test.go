//go:build integration

package worker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testWorkerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestIntegration_PruningWorker_runOnce(t *testing.T) {
	p := testWorkerPool(t)
	w := &PruningWorker{db: p, retentionDays: 30, interval: time.Hour}
	w.runOnce()
}

func TestIntegration_ExpiryWorker_runOnce(t *testing.T) {
	p := testWorkerPool(t)
	w := &ExpiryWorker{db: p, interval: time.Hour}
	w.runOnce(context.Background())
}
