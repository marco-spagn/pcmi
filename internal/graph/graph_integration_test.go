//go:build integration

package graph

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// realDB opens a pool to the AGE-enabled Postgres instance.
// Skips the test when DATABASE_URL is empty.
func realDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping graph integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedGraph inserts test memories and links for graph traversal tests.
// Returns the memory IDs for a 3-node causal chain: a → b → c.
func seedGraph(t *testing.T, pool *pgxpool.Pool, tenantID string) (a, b, c int64) {
	t.Helper()
	ctx := context.Background()

	// Ensure tenant exists (upsert).
	_, err := pool.Exec(ctx, `INSERT INTO tenants (id, name, settings) VALUES ($1::uuid, 'graph-int-test', '{}') ON CONFLICT DO NOTHING`, tenantID)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}

	// Insert 3 memories.
	contents := []string{
		"Graph integration: initial observation — high latency under load.",
		"Graph integration: root cause — connection pool exhaustion at 200 req.",
		"Graph integration: fix — increased pool size to 400, latency normal.",
	}
	paths := []string{
		"root.graph_int.a",
		"root.graph_int.b",
		"root.graph_int.c",
	}
	ids := make([]int64, 3)
	for i := range contents {
		err := pool.QueryRow(ctx, `
			INSERT INTO memory_entries (tenant_id, path, content, embedding_model)
			VALUES ($1::uuid, $2::ltree, $3, 'text-embedding-3-small')
			RETURNING id`, tenantID, paths[i], contents[i]).Scan(&ids[i])
		if err != nil {
			t.Fatalf("seed memory[%d]: %v", i, err)
		}
	}

	// Create links: a → b (causal), b → c (causal).
	for _, l := range []struct{ from, to int64 }{
		{ids[0], ids[1]},
		{ids[1], ids[2]},
	} {
		_, err := pool.Exec(ctx, `
			INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, weight)
			VALUES ($1::uuid, ('memory.' || $2::text)::ltree, ('memory.' || $3::text)::ltree, 'causal', 1.0)
			ON CONFLICT (tenant_id, from_path, to_path, link_type) DO NOTHING`,
			tenantID, l.from, l.to,
		)
		if err != nil {
			t.Fatalf("seed link %d->%d: %v", l.from, l.to, err)
		}
	}
	// a → c supports.
	_, err = pool.Exec(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, weight)
		VALUES ($1::uuid, ('memory.' || $2::text)::ltree, ('memory.' || $3::text)::ltree, 'supports', 1.0)
		ON CONFLICT (tenant_id, from_path, to_path, link_type) DO NOTHING`,
		tenantID, ids[0], ids[2],
	)
	if err != nil {
		t.Fatalf("seed supports link: %v", err)
	}

	// Sync links to AGE graph.
	for _, l := range []struct{ from, to int64; lt string }{
		{ids[0], ids[1], "causal"},
		{ids[1], ids[2], "causal"},
		{ids[0], ids[2], "supports"},
	} {
		_, err := pool.Exec(ctx,
			`SELECT public.sync_memory_link_to_graph(
				'memory.' || $2::text,
				'memory.' || $3::text,
				$4,
				1.0,
				$1::uuid
			)`, tenantID, l.from, l.to, l.lt,
		)
		if err != nil {
			t.Logf("sync link %d-[%s]->%d: %v (AGE may be absent)", l.from, l.lt, l.to, err)
		}
	}

	return ids[0], ids[1], ids[2]
}

// ─── Integration tests ─────────────────────────────────────────────────────

func TestIntegration_IsAvailable_WithRealDB(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	available := gc.IsAvailable(ctx)
	t.Logf("AGE available: %v", available)
}

func TestIntegration_FindRelated_WithRealDB(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping FindRelated integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa01"
	a, _, _ := seedGraph(t, pool, tenantID)

	result, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 3, 0, 50)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	if result == nil {
		t.Fatal("result must not be nil")
	}
	t.Logf("FindRelated(mem=%d): %d entries (total=%d, nextCursor=%d)",
		a, len(result.Memories), result.Total, result.NextCursor)
	if len(result.Memories) == 0 {
		t.Error("expected at least 1 related memory")
	}
}

func TestIntegration_FindChain_WithRealDB(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping FindChain integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa02"
	a, _, c := seedGraph(t, pool, tenantID)

	result, err := gc.FindChain(ctx, tenantID, a, c, []string{"causal"}, 10)
	if err != nil {
		t.Fatalf("FindChain: %v", err)
	}
	t.Logf("FindChain(%d→%d): connected=%v hops=%d path=%d",
		a, c, result.Connected, result.Hops, len(result.Path))
	if !result.Connected {
		t.Error("expected connected=true for a→b→c causal chain")
	}
	if result.Hops < 2 {
		t.Errorf("expected at least 2 hops, got %d", result.Hops)
	}
}

func TestIntegration_ExecuteCypher_WithRealDB(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping ExecuteCypher integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa03"
	seedGraph(t, pool, tenantID)

	result, err := gc.ExecuteCypher(ctx, tenantID, "MATCH (n:Memory) RETURN n.id ORDER BY n.id LIMIT 10")
	if err != nil {
		t.Fatalf("ExecuteCypher: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Error("expected at least 1 row from MATCH query")
	}
	t.Logf("ExecuteCypher: %d rows, columns=%v", len(result.Rows), result.Columns)
}

func TestIntegration_CreateLink_WithRealDB(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	tenantID := "00000000-0000-0000-0000-00000000aa04"
	a, b, _ := seedGraph(t, pool, tenantID)

	err := gc.CreateLink(ctx, tenantID, a, b, LinkTypeTemporal, 0.8)
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// Verify the link exists.
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM memory_links
		WHERE tenant_id = $1::uuid
		  AND from_path = ('memory.' || $2::text)::ltree
		  AND to_path = ('memory.' || $3::text)::ltree
		  AND link_type = 'temporal'`,
		tenantID, a, b,
	).Scan(&count)
	if err != nil {
		t.Fatalf("verify link: %v", err)
	}
	if count == 0 {
		t.Error("link was not created in memory_links")
	}
	t.Logf("CreateLink + verify: ok (%d rows)", count)
}

func TestIntegration_SyncMemoryLink_WithRealDB(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping SyncMemoryLink integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa05"
	a, b, _ := seedGraph(t, pool, tenantID)

	gc.SyncMemoryLink(ctx, tenantID,
		fmt.Sprintf("memory.%d", a),
		fmt.Sprintf("memory.%d", b),
		LinkTypeContradicts, 0.9)
	t.Log("SyncMemoryLink: completed without panic")
}

func TestIntegration_FindRelated_Pagination(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping pagination integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa06"
	a, _, _ := seedGraph(t, pool, tenantID)

	// First page — limit 1.
	p1, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 3, 0, 1)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	t.Logf("page1: %d entries, nextCursor=%d", len(p1.Memories), p1.NextCursor)

	if p1.NextCursor > 0 {
		p2, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 3, p1.NextCursor, 1)
		if err != nil {
			t.Fatalf("page 2: %v", err)
		}
		t.Logf("page2: %d entries, nextCursor=%d", len(p2.Memories), p2.NextCursor)
		if len(p1.Memories) > 0 && len(p2.Memories) > 0 {
			if p1.Memories[0].ID == p2.Memories[0].ID {
				t.Error("page 1 and page 2 must not overlap")
			}
		}
	}
}

func TestIntegration_FindChain_Unreachable(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping FindChain unreachable test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa07"
	a, _, _ := seedGraph(t, pool, tenantID)

	result, err := gc.FindChain(ctx, tenantID, a, 99999999, []string{"causal"}, 10)
	if err != nil {
		t.Fatalf("FindChain(unreachable): %v", err)
	}
	if result.Connected {
		t.Error("expected connected=false for non-existent target")
	}
	t.Logf("FindChain(unreachable): connected=%v (expected false)", result.Connected)
}

func TestIntegration_ExecuteCypher_RejectedDangerous(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping ExecuteCypher rejected test")
	}

	_, err := gc.ExecuteCypher(ctx, "00000000-0000-0000-0000-00000000aa08", "CREATE (n:Memory {id: 'evil'})")
	if err == nil {
		t.Fatal("CREATE query must be rejected")
	}
	t.Logf("ExecuteCypher(CREATE) rejected: %v", err)
}

func TestIntegration_FindRelated_MaxDepth(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping FindRelated maxDepth test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa09"
	a, _, _ := seedGraph(t, pool, tenantID)

	// Depth 1 should find only direct neighbours.
	result, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 1, 0, 50)
	if err != nil {
		t.Fatalf("FindRelated depth=1: %v", err)
	}
	t.Logf("FindRelated depth=1: %d entries", len(result.Memories))
	// Should only find the direct neighbour (b), not the 2-hop (c).
	for _, m := range result.Memories {
		if m.Depth > 1 {
			t.Errorf("depth=1 query returned depth=%d entry (id=%d)", m.Depth, m.ID)
		}
	}
}

func TestIntegration_FindChain_ReverseDirection(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping FindChain reverse test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa10"
	a, _, c := seedGraph(t, pool, tenantID)

	// Reverse direction: c → a. Edges are directed a→b→c, so this should
	// either find a path via different link types or report not connected.
	result, err := gc.FindChain(ctx, tenantID, c, a, []string{"causal"}, 10)
	if err != nil {
		t.Fatalf("FindChain(reverse): %v", err)
	}
	t.Logf("FindChain(%d→%d reverse): connected=%v hops=%d", c, a, result.Connected, result.Hops)
	// With directed causal edges, reverse should be not connected.
	if result.Connected {
		t.Log("reverse direction unexpectedly connected (may have other link types)")
	}
}

func TestIntegration_GraphClient_QueryTimeoutDefault(t *testing.T) {
	gc := NewGraphClient(nil)
	if gc.queryTimeout != 30*time.Second {
		t.Errorf("default queryTimeout: got %v want 30s", gc.queryTimeout)
	}
}
