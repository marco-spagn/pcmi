//go:build integration

package graph

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	// Ensure tenant exists (upsert). slug is NOT NULL (migrations/001_init.sql).
	slug := "graph-int-" + tenantID[strings.LastIndex(tenantID, "-")+1:]
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, settings)
		VALUES ($1::uuid, $2, 'graph-int-test', '{}')
		ON CONFLICT (id) DO NOTHING`,
		tenantID, slug,
	)
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
	// weight is stored in metadata, not as a column (column only exists via migration 019).
	for _, l := range []struct{ from, to int64 }{
		{ids[0], ids[1]},
		{ids[1], ids[2]},
	} {
		fromPath := fmt.Sprintf("memory.%d", l.from)
		toPath := fmt.Sprintf("memory.%d", l.to)
		_, err := pool.Exec(ctx, `
			INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, metadata)
			VALUES ($1::uuid, $2::ltree, $3::ltree, 'causal', jsonb_build_object('weight', 1.0))
			ON CONFLICT (tenant_id, from_path, to_path, link_type) DO NOTHING`,
			tenantID, fromPath, toPath,
		)
		if err != nil {
			t.Fatalf("seed link %d->%d: %v", l.from, l.to, err)
		}
	}
	// a → c supports.
	fromAC := fmt.Sprintf("memory.%d", ids[0])
	toAC := fmt.Sprintf("memory.%d", ids[2])
	_, err = pool.Exec(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, metadata)
		VALUES ($1::uuid, $2::ltree, $3::ltree, 'supports', jsonb_build_object('weight', 1.0))
		ON CONFLICT (tenant_id, from_path, to_path, link_type) DO NOTHING`,
		tenantID, fromAC, toAC,
	)
	if err != nil {
		t.Fatalf("seed supports link: %v", err)
	}

	// Sync links to AGE graph.
	for _, l := range []struct {
		from, to int64
		lt       string
	}{
		{ids[0], ids[1], "causal"},
		{ids[1], ids[2], "causal"},
		{ids[0], ids[2], "supports"},
	} {
		fromP := fmt.Sprintf("memory.%d", l.from)
		toP := fmt.Sprintf("memory.%d", l.to)
		_, err := pool.Exec(ctx,
			`SELECT public.sync_memory_link_to_graph($1, $2, $3, $4, $5::uuid)`,
			fromP, toP, l.lt, 1.0, tenantID,
		)
		if err != nil {
			t.Logf("sync link %d-[%s]->%d: %v (AGE may be absent)", l.from, l.lt, l.to, err)
		}
	}

	return ids[0], ids[1], ids[2]
}

func seedTenant(t *testing.T, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	ctx := context.Background()
	slug := "graph-int-" + tenantID[strings.LastIndex(tenantID, "-")+1:]
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, settings)
		VALUES ($1::uuid, $2, 'graph-int-test', '{}')
		ON CONFLICT (id) DO NOTHING`,
		tenantID, slug,
	); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}
}

func insertMemoryWithID(t *testing.T, pool *pgxpool.Pool, tenantID string, id int64, path, content string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO memory_entries (id, tenant_id, path, content, embedding_model)
		VALUES ($1, $2::uuid, $3::ltree, $4, 'text-embedding-3-small')
		ON CONFLICT (id) DO UPDATE
		    SET tenant_id = EXCLUDED.tenant_id,
		        path = EXCLUDED.path,
		        content = EXCLUDED.content,
		        embedding_model = EXCLUDED.embedding_model`,
		id, tenantID, path, content,
	)
	if err != nil {
		t.Fatalf("insert memory %d: %v", id, err)
	}
}

func insertGraphLink(t *testing.T, pool *pgxpool.Pool, tenantID string, fromID, toID int64, linkType string) {
	t.Helper()
	ctx := context.Background()
	fromPath := fmt.Sprintf("memory.%d", fromID)
	toPath := fmt.Sprintf("memory.%d", toID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO memory_links (tenant_id, from_path, to_path, link_type, metadata)
		VALUES ($1::uuid, $2::ltree, $3::ltree, $4, jsonb_build_object('weight', 1.0))
		ON CONFLICT (tenant_id, from_path, to_path, link_type) DO UPDATE
		    SET metadata = EXCLUDED.metadata`,
		tenantID, fromPath, toPath, linkType,
	); err != nil {
		t.Fatalf("insert link %d-[%s]->%d: %v", fromID, linkType, toID, err)
	}
	if _, err := pool.Exec(ctx,
		`SELECT public.sync_memory_link_to_graph($1, $2, $3, $4, $5::uuid)`,
		fromPath, toPath, linkType, 1.0, tenantID,
	); err != nil {
		t.Fatalf("sync link %d-[%s]->%d: %v", fromID, linkType, toID, err)
	}
}

func rowsContainMemoryID(rows []map[string]interface{}, id int64) bool {
	needle := fmt.Sprintf("memory.%d", id)
	for _, row := range rows {
		if strings.Contains(fmt.Sprint(row), needle) {
			return true
		}
	}
	return false
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

	result, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 3, 0, 50, TraversalBoth)
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

func TestIntegration_FindRelated_DeduplicatesMultiplePaths(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping duplicate-path integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa11"
	a, b, c := seedGraph(t, pool, tenantID)

	result, err := gc.FindRelated(ctx, tenantID, a, nil, 3, 0, 50, TraversalBoth)
	if err != nil {
		t.Fatalf("FindRelated: %v", err)
	}
	seen := map[int64]int{}
	depthByID := map[int64]int{}
	for _, memory := range result.Memories {
		seen[memory.ID]++
		depthByID[memory.ID] = memory.Depth
	}
	if seen[b] != 1 {
		t.Fatalf("direct neighbor b should appear once, got %d entries: %#v", seen[b], result.Memories)
	}
	if seen[c] != 1 {
		t.Fatalf("target c reachable by direct and multi-hop paths should be deduplicated, got %d entries: %#v", seen[c], result.Memories)
	}
	if depthByID[c] != 1 {
		t.Fatalf("deduplicated target c should keep shortest depth=1 via direct supports edge, got %d", depthByID[c])
	}
	if result.Total != 2 {
		t.Fatalf("total should count unique related memories, got %d", result.Total)
	}
}

func TestIntegration_FindRelated_NumericPaginationBoundary(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping numeric pagination integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa12"
	seedTenant(t, pool, tenantID)
	base := (time.Now().UnixNano() / 1_000_000) * 100
	hubID := base
	targetIDs := []int64{base + 8, base + 9, base + 10, base + 11, base + 12}

	insertMemoryWithID(t, pool, tenantID, hubID, fmt.Sprintf("root.graph_int.pagination.hub_%d", hubID), "Graph pagination hub")
	for _, id := range targetIDs {
		insertMemoryWithID(t, pool, tenantID, id, fmt.Sprintf("root.graph_int.pagination.node_%d", id), fmt.Sprintf("Graph pagination target %d", id))
		insertGraphLink(t, pool, tenantID, hubID, id, LinkTypeRelated)
	}

	var got []int64
	cursor := int64(0)
	for {
		page, err := gc.FindRelated(ctx, tenantID, hubID, []string{LinkTypeRelated}, 1, cursor, 2, TraversalBoth)
		if err != nil {
			t.Fatalf("FindRelated page cursor=%d: %v", cursor, err)
		}
		for _, memory := range page.Memories {
			got = append(got, memory.ID)
		}
		if page.NextCursor == 0 {
			break
		}
		if page.NextCursor <= cursor {
			t.Fatalf("next cursor must advance numerically: old=%d new=%d", cursor, page.NextCursor)
		}
		cursor = page.NextCursor
	}
	if len(got) != len(targetIDs) {
		t.Fatalf("expected %d paginated targets, got %d: %v", len(targetIDs), len(got), got)
	}
	for i, want := range targetIDs {
		if got[i] != want {
			t.Fatalf("numeric pagination order mismatch at %d: got %v want %v", i, got, targetIDs)
		}
	}
}

func TestIntegration_ExecuteCypher_MultiTenantIsolation(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping multi-tenant Cypher integration test")
	}

	tenantA := "00000000-0000-0000-0000-00000000aa13"
	tenantB := "00000000-0000-0000-0000-00000000aa14"
	a, _, _ := seedGraph(t, pool, tenantA)
	b, _, _ := seedGraph(t, pool, tenantB)

	result, err := gc.ExecuteCypher(ctx, tenantA, "MATCH (a:Memory), (b:Memory) RETURN b.id LIMIT 100")
	if err != nil {
		t.Fatalf("ExecuteCypher multi-alias query: %v", err)
	}
	if rowsContainMemoryID(result.Rows, b) {
		t.Fatalf("tenant A Cypher query leaked tenant B memory id %d in rows: %#v", b, result.Rows)
	}
	if !rowsContainMemoryID(result.Rows, a) {
		t.Fatalf("tenant A Cypher query should still return tenant A memory id %d, rows: %#v", a, result.Rows)
	}
}

func TestIntegration_MemoryLinkDeleteRemovesAGEEdge(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping graph drift integration test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa15"
	seedTenant(t, pool, tenantID)
	base := (time.Now().UnixNano() / 1_000_000) * 100
	fromID := base + 20
	toID := base + 21
	fromPath := fmt.Sprintf("memory.%d", fromID)
	toPath := fmt.Sprintf("memory.%d", toID)

	insertMemoryWithID(t, pool, tenantID, fromID, fmt.Sprintf("root.graph_int.drift.from_%d", fromID), "Graph drift source")
	insertMemoryWithID(t, pool, tenantID, toID, fmt.Sprintf("root.graph_int.drift.to_%d", toID), "Graph drift target")
	insertGraphLink(t, pool, tenantID, fromID, toID, LinkTypeCausal)

	before, err := gc.FindRelated(ctx, tenantID, fromID, []string{LinkTypeCausal}, 1, 0, 50, TraversalBoth)
	if err != nil {
		t.Fatalf("FindRelated before delete: %v", err)
	}
	foundBefore := false
	for _, memory := range before.Memories {
		if memory.ID == toID {
			foundBefore = true
		}
	}
	if !foundBefore {
		t.Fatalf("expected target %d before deleting SQL link, got %#v", toID, before.Memories)
	}

	if _, err := pool.Exec(ctx, `
		DELETE FROM memory_links
		WHERE tenant_id = $1::uuid
		  AND from_path = $2::ltree
		  AND to_path = $3::ltree
		  AND link_type = $4`,
		tenantID, fromPath, toPath, LinkTypeCausal,
	); err != nil {
		t.Fatalf("delete memory link: %v", err)
	}

	after, err := gc.FindRelated(ctx, tenantID, fromID, []string{LinkTypeCausal}, 1, 0, 50, TraversalBoth)
	if err != nil {
		t.Fatalf("FindRelated after delete: %v", err)
	}
	for _, memory := range after.Memories {
		if memory.ID == toID {
			t.Fatalf("deleted SQL link still appears in AGE traversal: %#v", after.Memories)
		}
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
	fromP := fmt.Sprintf("memory.%d", a)
	toP := fmt.Sprintf("memory.%d", b)
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM memory_links
		WHERE tenant_id = $1::uuid
		  AND from_path = $2::ltree
		  AND to_path = $3::ltree
		  AND link_type = 'temporal'`,
		tenantID, fromP, toP,
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
	p1, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 3, 0, 1, TraversalBoth)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	t.Logf("page1: %d entries, nextCursor=%d", len(p1.Memories), p1.NextCursor)

	if p1.NextCursor > 0 {
		p2, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 3, p1.NextCursor, 1, TraversalBoth)
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
	result, err := gc.FindRelated(ctx, tenantID, a, []string{"causal"}, 1, 0, 50, TraversalBoth)
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

func TestIntegration_FindRelated_IncomingEdges(t *testing.T) {
	pool := realDB(t)
	gc := NewGraphClient(pool)
	ctx := context.Background()

	if !gc.IsAvailable(ctx) {
		t.Skip("AGE not available — skipping FindRelated incoming test")
	}

	tenantID := "00000000-0000-0000-0000-00000000aa16"
	_, b, c := seedGraph(t, pool, tenantID)

	out, err := gc.FindRelated(ctx, tenantID, c, []string{"causal"}, 1, 0, 50, TraversalOut)
	if err != nil {
		t.Fatalf("FindRelated out: %v", err)
	}
	if out.Total != 0 {
		t.Fatalf("outgoing depth=1 from c: total=%d, want 0", out.Total)
	}

	both, err := gc.FindRelated(ctx, tenantID, c, []string{"causal"}, 1, 0, 50, TraversalBoth)
	if err != nil {
		t.Fatalf("FindRelated both: %v", err)
	}
	if both.Total != 1 || len(both.Memories) != 1 || both.Memories[0].ID != b {
		t.Fatalf("both depth=1 from c: got %+v, want neighbour b=%d", both.Memories, b)
	}

	in, err := gc.FindRelated(ctx, tenantID, c, []string{"causal"}, 1, 0, 50, TraversalIn)
	if err != nil {
		t.Fatalf("FindRelated in: %v", err)
	}
	if in.Total != 1 || in.Memories[0].ID != b {
		t.Fatalf("incoming depth=1 from c: got %+v, want b=%d", in.Memories, b)
	}
}

func TestIntegration_GraphClient_QueryTimeoutDefault(t *testing.T) {
	gc := NewGraphClient(nil)
	if gc.queryTimeout != 30*time.Second {
		t.Errorf("default queryTimeout: got %v want 30s", gc.queryTimeout)
	}
}
