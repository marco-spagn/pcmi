//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/usage"
)

func TestIntegration_Usage_aggregatorFlushIsAdditive(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	tenantID := newRetentionTenant(t, ctx, pool)
	agg := usage.NewAggregator(pool)
	day := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 2; i++ { // two flushes of the same bucket must add up
		agg.Record(usage.Event{TenantID: tenantID, Operation: usage.OpEmbedding, Provider: "openai", Model: "emb", InputTokens: 100, At: day})
		agg.Record(usage.Event{TenantID: tenantID, Operation: usage.OpDistillation, Provider: "anthropic", Model: "claude", InputTokens: 10, OutputTokens: 5, At: day.Add(24 * time.Hour)})
		if err := agg.Flush(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := NewUsageRepository(pool).Daily(ctx, tenantID, day, day.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows %+v", rows)
	}
	if r := rows[0]; r.Day != "2026-09-01" || r.Operation != usage.OpEmbedding || r.Requests != 2 || r.InputTokens != 200 {
		t.Fatalf("row0 %+v", r)
	}
	if r := rows[1]; r.Day != "2026-09-02" || r.Requests != 2 || r.OutputTokens != 10 {
		t.Fatalf("row1 %+v", r)
	}
	only, err := NewUsageRepository(pool).Daily(ctx, tenantID, day, day)
	if err != nil || len(only) != 1 {
		t.Fatalf("window filter: %v %v", only, err)
	}
}

func TestIntegration_Usage_poisonBucketDoesNotBlockOthers(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	tenantID := newRetentionTenant(t, ctx, pool)
	agg := usage.NewAggregator(pool)
	now := time.Now().UTC()
	// Unknown tenant violates the FK; it must be dropped, not block the good row.
	agg.Record(usage.Event{TenantID: "00000000-0000-0000-0000-00000000dead", Operation: "x", Provider: "p", Model: "m", At: now})
	agg.Record(usage.Event{TenantID: tenantID, Operation: usage.OpRerank, Provider: "p", Model: "m", InputTokens: 9, At: now})
	if err := agg.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if agg.Pending() != 0 {
		t.Fatalf("pending %d", agg.Pending())
	}
	rows, err := NewUsageRepository(pool).Daily(ctx, tenantID, now, now)
	if err != nil || len(rows) != 1 || rows[0].InputTokens != 9 {
		t.Fatalf("rows %+v err %v", rows, err)
	}
}
