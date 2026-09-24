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
	w.runOnce()
}

// TestIntegration_markRunCompleted_updatesRun runs the real SQL: the query used
// to bind an unreferenced $2, which Postgres rejects (42P18), so runs stayed
// 'running' forever.
func TestIntegration_markRunCompleted_updatesRun(t *testing.T) {
	ctx := context.Background()
	p := testWorkerPool(t)
	var tenantID string
	slug := "mark-run-" + time.Now().Format("150405.000000000")
	if err := p.QueryRow(ctx, `INSERT INTO tenants (name, slug) VALUES ($1, $1) RETURNING id::text`, slug).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	var policyID, runID, distilledID int64
	if err := p.QueryRow(ctx, `INSERT INTO distillation_policies (tenant_id, name, path_prefix) VALUES ($1::uuid, 'mark', 'root.mark') RETURNING id`, tenantID).Scan(&policyID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO distillation_runs (policy_id, tenant_id, path_prefix, status) VALUES ($1, $2::uuid, 'root.mark', 'running') RETURNING id`, policyID, tenantID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO distilled_knowledge (tenant_id, path, summary, source_entry_ids) VALUES ($1::uuid, 'root.mark.distilled', 's', ARRAY[1]::bigint[]) RETURNING id`, tenantID).Scan(&distilledID); err != nil {
		t.Fatal(err)
	}
	(&DistillationWorker{db: p}).markRunCompleted(ctx, tenantID, distilledID)
	var status string
	var gotDistilled *int64
	if err := p.QueryRow(ctx, `SELECT status, distilled_id FROM distillation_runs WHERE id = $1`, runID).Scan(&status, &gotDistilled); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || gotDistilled == nil || *gotDistilled != distilledID {
		t.Fatalf("status=%s distilled_id=%v", status, gotDistilled)
	}
}
