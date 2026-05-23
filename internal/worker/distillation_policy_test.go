package worker

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestDistillationPolicy_TriggerOnCount_OverThreshold(t *testing.T) {
	p := model.DistillationPolicy{Enabled: true, CountThreshold: 5, MinIntervalSecs: 0}
	st := PolicyEvalState{MemoryCount: 6}
	ok, reason := ShouldTriggerPolicy(p, st, time.Now())
	if !ok || reason != "count_threshold" {
		t.Fatalf("expected count trigger, got ok=%v reason=%q", ok, reason)
	}
}

func TestDistillationPolicy_NoTrigger_UnderThreshold(t *testing.T) {
	p := model.DistillationPolicy{Enabled: true, CountThreshold: 10, MinIntervalSecs: 0}
	st := PolicyEvalState{MemoryCount: 3}
	ok, reason := ShouldTriggerPolicy(p, st, time.Now())
	if ok || reason != "under_threshold" {
		t.Fatalf("expected no trigger, got ok=%v reason=%q", ok, reason)
	}
}

func TestDistillationPolicy_RespectsMinInterval(t *testing.T) {
	last := time.Now().Add(-30 * time.Second)
	p := model.DistillationPolicy{
		Enabled:         true,
		CountThreshold:  1,
		MinIntervalSecs: 300,
		LastTriggeredAt: &last,
	}
	st := PolicyEvalState{MemoryCount: 100}
	ok, reason := ShouldTriggerPolicy(p, st, time.Now())
	if ok || reason != "min_interval" {
		t.Fatalf("expected min_interval block, got ok=%v reason=%q", ok, reason)
	}
}

func TestDistillationPolicy_AgeBasedTrigger(t *testing.T) {
	maxAge := 3600
	p := model.DistillationPolicy{
		Enabled:         true,
		CountThreshold:  1000,
		MinIntervalSecs: 0,
		MaxAgeSecs:      &maxAge,
	}
	st := PolicyEvalState{MemoryCount: 1, OldestEntryAge: 2 * time.Hour}
	ok, reason := ShouldTriggerPolicy(p, st, time.Now())
	if !ok || reason != "max_age" {
		t.Fatalf("expected max_age trigger, got ok=%v reason=%q", ok, reason)
	}
}

func policyTestPool(t *testing.T) *pgxpool.Pool {
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
	applyMigration018(t, pool)
	return pool
}

func testPolicyPool(t *testing.T) *DistillationPolicyEngine {
	t.Helper()
	return NewDistillationPolicyEngine(policyTestPool(t), nil)
}

func applyMigration018(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	sql, err := os.ReadFile("migrations/018_distillation_policy.sql")
	if err != nil {
		sql, err = os.ReadFile("../../migrations/018_distillation_policy.sql")
	}
	if err != nil {
		t.Fatalf("read migration 018: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("apply migration 018: %v", err)
		}
	}
}

func TestDistillationRun_RecordedInDB(t *testing.T) {
	engine := testPolicyPool(t)
	ctx := context.Background()
	tenantID := "00000000-0000-0000-0000-000000000000"
	_, _ = engine.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID)

	p, err := InsertPolicyForTest(ctx, engine.db, tenantID, model.CreateDistillationPolicyRequest{
		Name:           "run-record-" + time.Now().Format("150405"),
		PathPrefix:     "root.policy.run",
		CountThreshold: 1,
		MinIntervalSecs: 0,
	})
	if err != nil {
		t.Fatalf("insert policy: %v", err)
	}
	runID, err := engine.beginRun(ctx, p, 3)
	if err != nil {
		t.Fatalf("begin run: %v", err)
	}
	if err := engine.completeRun(ctx, runID, "completed", nil); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	run, err := GetLatestRunForPolicy(ctx, engine.db, p.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "completed" || run.SourceCount != 3 {
		t.Fatalf("unexpected run: %+v", run)
	}
}

func TestDistillationRun_FailureRecorded_WithError(t *testing.T) {
	engine := testPolicyPool(t)
	ctx := context.Background()
	tenantID := "00000000-0000-0000-0000-000000000000"
	_, _ = engine.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID)

	p, err := InsertPolicyForTest(ctx, engine.db, tenantID, model.CreateDistillationPolicyRequest{
		Name:            "run-fail-" + time.Now().Format("150405"),
		PathPrefix:      "root.policy.fail",
		CountThreshold:  1,
		MinIntervalSecs: 0,
	})
	if err != nil {
		t.Fatalf("insert policy: %v", err)
	}
	runID, err := engine.beginRun(ctx, p, 1)
	if err != nil {
		t.Fatalf("begin run: %v", err)
	}
	msg := "llm timeout"
	if err := engine.RecordRunFailure(ctx, runID, msg); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	run, err := GetLatestRunForPolicy(ctx, engine.db, p.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if run.Status != "failed" || run.ErrorMessage == nil || *run.ErrorMessage != msg {
		t.Fatalf("unexpected run: %+v", run)
	}
}
