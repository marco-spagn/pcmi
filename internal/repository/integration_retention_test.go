//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/auditchain"
	"github.com/marco-spagn/pcmi/internal/model"
)

func newRetentionTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	slug := fmt.Sprintf("retention-%d", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, `INSERT INTO tenants (name, slug) VALUES ($1, $1) RETURNING id::text`, slug).Scan(&id); err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return id
}

// insertMemory writes one version row; validTo/validFrom offsets are relative to NOW().
func insertMemory(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, path string, version int, validFromAgo, validToAgo time.Duration, closed bool) int64 {
	t.Helper()
	var id int64
	var validTo any
	if closed {
		validTo = time.Now().Add(-validToAgo)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO memory_entries (tenant_id, path, content, embedding_model, version, valid_from, valid_to)
		VALUES ($1::uuid, $2::ltree, $3, 'test', $4, $5, $6) RETURNING id`,
		tenantID, path, "content of "+path, version, time.Now().Add(-validFromAgo), validTo).Scan(&id); err != nil {
		t.Fatalf("insert %s: %v", path, err)
	}
	return id
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", q, err)
	}
	return n
}

func TestIntegration_Retention_policyCRUD(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	repo := NewRetentionRepository(pool)
	tenantID := newRetentionTenant(t, ctx, pool)
	d := func(n int) *int { return &n }

	p, err := repo.Upsert(ctx, tenantID, model.RetentionPolicyRequest{PathPrefix: "finance", SupersededRetentionDays: d(2555)})
	if err != nil || p.PathPrefix != "finance" || *p.SupersededRetentionDays != 2555 || p.MaxAgeDays != nil {
		t.Fatalf("upsert: %+v %v", p, err)
	}
	p, err = repo.Upsert(ctx, tenantID, model.RetentionPolicyRequest{PathPrefix: "finance", MaxAgeDays: d(10)})
	if err != nil || p.SupersededRetentionDays != nil || *p.MaxAgeDays != 10 {
		t.Fatalf("replace: %+v %v", p, err)
	}
	if _, err := repo.Upsert(ctx, tenantID, model.RetentionPolicyRequest{PathPrefix: "", SupersededRetentionDays: d(7)}); err != nil {
		t.Fatalf("tenant-wide: %v", err)
	}
	list, err := repo.List(ctx, tenantID)
	if err != nil || len(list) != 2 || list[0].PathPrefix != "" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := repo.Delete(ctx, tenantID, "finance"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, tenantID, "finance"); err != ErrRetentionPolicyNotFound {
		t.Fatalf("second delete: %v", err)
	}
}

func TestIntegration_Retention_pruneAndExpireHonourPolicies(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	repo := NewRetentionRepository(pool)
	tenantID := newRetentionTenant(t, ctx, pool)
	d := func(n int) *int { return &n }
	day := 24 * time.Hour

	// finance keeps superseded versions 10y; finance.scratch only 1 day; tmp: current rows max 5 days.
	for _, req := range []model.RetentionPolicyRequest{
		{PathPrefix: "finance", SupersededRetentionDays: d(3650)},
		{PathPrefix: "finance.scratch", SupersededRetentionDays: d(1)},
		{PathPrefix: "tmp", MaxAgeDays: d(5)},
	} {
		if _, err := repo.Upsert(ctx, tenantID, req); err != nil {
			t.Fatal(err)
		}
	}
	oldFinance := insertMemory(t, ctx, pool, tenantID, "finance.ledger", 1, 400*day, 60*day, true)
	oldScratch := insertMemory(t, ctx, pool, tenantID, "finance.scratch.x", 1, 10*day, 2*day, true)
	oldDefault := insertMemory(t, ctx, pool, tenantID, "notes.a", 1, 100*day, 60*day, true)
	freshDefault := insertMemory(t, ctx, pool, tenantID, "notes.b", 1, 10*day, 2*day, true)
	agedTmp := insertMemory(t, ctx, pool, tenantID, "tmp.old", 1, 9*day, 0, false)
	youngTmp := insertMemory(t, ctx, pool, tenantID, "tmp.new", 1, 1*day, 0, false)
	agedOther := insertMemory(t, ctx, pool, tenantID, "notes.current", 1, 900*day, 0, false)

	var pruned, closed int
	if err := pool.QueryRow(ctx, `SELECT prune_superseded_memories_with_policies(30)`).Scan(&pruned); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT expire_memories_by_retention_policy()`).Scan(&closed); err != nil {
		t.Fatal(err)
	}
	exists := func(id int64) bool {
		return countRows(t, ctx, pool, `SELECT count(*) FROM memory_entries WHERE id = $1`, id) == 1
	}
	isOpen := func(id int64) bool {
		return countRows(t, ctx, pool, `SELECT count(*) FROM memory_entries WHERE id = $1 AND valid_to IS NULL`, id) == 1
	}
	if !exists(oldFinance) {
		t.Error("finance (3650d policy) superseded row must be kept past the 30d default")
	}
	if exists(oldScratch) {
		t.Error("finance.scratch (1d, most specific) row must be pruned")
	}
	if exists(oldDefault) {
		t.Error("row without policy older than 30d default must be pruned")
	}
	if !exists(freshDefault) {
		t.Error("row without policy younger than default must be kept")
	}
	if isOpen(agedTmp) {
		t.Error("tmp row older than max_age_days must be closed")
	}
	if !isOpen(youngTmp) || !isOpen(agedOther) {
		t.Error("rows within max_age or outside any max_age policy must stay open")
	}
	if pruned < 2 || closed < 1 {
		t.Fatalf("pruned=%d closed=%d", pruned, closed)
	}
}

func TestIntegration_Retention_eraseRemovesEverythingAndAudits(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	repo := NewRetentionRepository(pool)
	tenantID := newRetentionTenant(t, ctx, pool)
	other := newRetentionTenant(t, ctx, pool)
	day := 24 * time.Hour

	a1 := insertMemory(t, ctx, pool, tenantID, "users.alice.profile", 1, 3*day, 2*day, true)
	a2 := insertMemory(t, ctx, pool, tenantID, "users.alice.profile", 2, 2*day, 0, false)
	insertMemory(t, ctx, pool, tenantID, "users.alice.notes", 1, day, 0, false)
	bob := insertMemory(t, ctx, pool, tenantID, "users.bob.profile", 1, day, 0, false)
	insertMemory(t, ctx, pool, tenantID, "users.alice_smith.x", 1, day, 0, false) // sibling label, must survive
	insertMemory(t, ctx, pool, other, "users.alice.profile", 1, day, 0, false)   // other tenant, must survive

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO memory_links (tenant_id, from_path, to_path, link_type) VALUES ($1::uuid, 'users.bob.profile', 'users.alice.profile', 'related')`, tenantID)
	mustExec(`INSERT INTO memory_links (tenant_id, from_path, to_path, link_type) VALUES ($1::uuid, 'users.bob.profile', 'users.alice_smith.x', 'related')`, tenantID)
	mustExec(`INSERT INTO distilled_knowledge (tenant_id, path, summary, source_entry_ids) VALUES ($1::uuid, 'digests.weekly', 'mentions alice', ARRAY[$2::bigint, $3::bigint])`, tenantID, a2, bob)
	mustExec(`INSERT INTO distilled_knowledge (tenant_id, path, summary, source_entry_ids) VALUES ($1::uuid, 'digests.bob', 'bob only', ARRAY[$2::bigint])`, tenantID, bob)
	mustExec(`INSERT INTO graph_link_proposals (tenant_id, source_memory_id, from_memory_id, to_memory_id, from_path, to_path, link_type) VALUES ($1::uuid, $2, $2, $3, 'users.bob.profile', 'users.bob.profile', 'related')`, tenantID, a1, bob)
	mustExec(`INSERT INTO consolidation_runs (tenant_id, path_prefix, source_entry_ids, consolidated_path) VALUES ($1::uuid, 'users', ARRAY[$2::bigint], 'users.summary')`, tenantID, a1)
	mustExec(`INSERT INTO consolidation_runs (tenant_id, path_prefix, source_entry_ids, consolidated_path) VALUES ($1::uuid, 'users.alice', ARRAY[]::bigint[], 'users.alice.summary')`, tenantID)

	// Dry run: exact counts, nothing removed.
	dry, ids, err := repo.Erase(ctx, tenantID, model.EraseRequest{PathPrefix: "users.alice", DryRun: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := model.EraseCounts{MemoryVersions: 3, Paths: 2, Links: 1, Distilled: 1, LinkProposals: 1, ConsolidationRuns: 2}
	if dry != want || ids != nil {
		t.Fatalf("dry-run counts %+v want %+v (ids=%v)", dry, want, ids)
	}
	if countRows(t, ctx, pool, `SELECT count(*) FROM memory_entries WHERE tenant_id = $1::uuid AND path <@ 'users.alice'`, tenantID) != 3 {
		t.Fatal("dry run must not delete")
	}

	auditBefore := countRows(t, ctx, pool, `SELECT count(*) FROM audit_log WHERE tenant_id = $1::uuid AND event_type = 'gdpr_erase'`, tenantID)
	got, ids, err := repo.Erase(ctx, tenantID, model.EraseRequest{PathPrefix: "users.alice", Reason: "DSR-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("counts %+v want %+v", got, want)
	}
	if len(ids) != 5 { // 2 paths + 3 memory.<id>
		t.Fatalf("vertex ids %v", ids)
	}
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM memory_entries WHERE tenant_id = $1::uuid AND path <@ 'users.alice'`, tenantID); n != 0 {
		t.Fatalf("%d alice rows survived", n)
	}
	if countRows(t, ctx, pool, `SELECT count(*) FROM memory_entries WHERE tenant_id = $1::uuid`, tenantID) != 2 {
		t.Fatal("bob and alice_smith rows must survive")
	}
	if countRows(t, ctx, pool, `SELECT count(*) FROM memory_entries WHERE tenant_id = $1::uuid`, other) != 1 {
		t.Fatal("other tenant must be untouched")
	}
	if countRows(t, ctx, pool, `SELECT count(*) FROM distilled_knowledge WHERE tenant_id = $1::uuid`, tenantID) != 1 {
		t.Fatal("only the bob-only digest must survive")
	}
	if countRows(t, ctx, pool, `SELECT count(*) FROM memory_links WHERE tenant_id = $1::uuid`, tenantID) != 1 {
		t.Fatal("only the bob→alice_smith link must survive")
	}
	if countRows(t, ctx, pool, `SELECT count(*) FROM audit_log WHERE tenant_id = $1::uuid AND event_type = 'gdpr_erase'`, tenantID) != auditBefore+1 {
		t.Fatal("erase must append one gdpr_erase audit row")
	}

	// The erasure evidence is part of the verified audit chain.
	recs, err := NewAuditRepository(pool).ChainBatch(ctx, tenantID, 0, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	v := auditchain.NewVerifier(tenantID)
	for _, r := range recs {
		v.Add(r)
	}
	if !v.Valid() {
		t.Fatalf("audit chain broken after erase: %+v", v.FirstBreak)
	}

	// Without the AGE helper (migration 028 skips it when AGE is absent) graph
	// cleanup is a no-op; with it, the call must still succeed.
	var helper *string
	if err := pool.QueryRow(ctx, `SELECT to_regproc('public.erase_memory_graph_vertices')::text`).Scan(&helper); err != nil {
		t.Fatal(err)
	}
	n, err := repo.EraseGraphVertices(ctx, tenantID, []string{"users.alice.profile"})
	if err != nil || (helper == nil && n != 0) {
		t.Fatalf("graph cleanup: helper=%v n=%d err=%v", helper != nil, n, err)
	}
}

// TestIntegration_Retention_eraseGraphVertices needs an AGE-enabled database
// (docker compose --profile graph) in PCMI_AGE_DATABASE_URL.
func TestIntegration_Retention_eraseGraphVertices(t *testing.T) {
	url := os.Getenv("PCMI_AGE_DATABASE_URL")
	if url == "" {
		t.Skip("PCMI_AGE_DATABASE_URL not set — skipping AGE erase test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	repo := NewRetentionRepository(pool)
	tenantID := newRetentionTenant(t, ctx, pool)
	if _, err := pool.Exec(ctx, `INSERT INTO memory_links (tenant_id, from_path, to_path, link_type)
		VALUES ($1::uuid, 'users.alice.notes', 'users.bob', 'related')`, tenantID); err != nil {
		t.Fatal(err)
	}
	countVertices := func() int {
		t.Helper()
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		if _, err := conn.Exec(ctx, `SET search_path = ag_catalog, "$user", public`); err != nil {
			t.Fatal(err)
		}
		var n int
		q := fmt.Sprintf(`SELECT c::text::int FROM cypher('pcmi_memory_graph', $$ MATCH (m:Memory {tenant_id: '%s'}) RETURN count(m) $$) AS (c agtype)`, tenantID)
		if err := conn.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := countVertices(); n != 2 {
		t.Fatalf("setup: %d vertices", n)
	}
	if _, err := repo.EraseGraphVertices(ctx, tenantID, []string{"users.alice.notes", "memory.1"}); err != nil {
		t.Fatal(err)
	}
	if n := countVertices(); n != 1 {
		t.Fatalf("after erase: %d vertices, want 1", n)
	}
}
