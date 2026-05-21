//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	pcmicrypto "github.com/marco-spagn/pcmi/internal/crypto"
	"github.com/marco-spagn/pcmi/internal/model"
)

func testDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — skipping integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func setTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}
}

func TestIntegration_MemoryRepository_flow(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	tenantID := "00000000-0000-0000-0000-000000000000"
	setTenant(t, ctx, pool, tenantID)

	t.Cleanup(pcmicrypto.ResetKey)
	if err := pcmicrypto.InitKey("01234567890123456789012345678901"); err != nil {
		t.Fatal(err)
	}

	path := fmt.Sprintf("root.integration.%d", time.Now().UnixNano())
	repo := NewMemoryRepository(pool, pool)

	id1, ver1, _, err := repo.Store(ctx, model.StoreRequest{Path: path, Content: "alpha beta gamma for search"}, tenantID)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if id1 <= 0 || ver1 != 1 {
		t.Fatalf("first store: id=%d ver=%d", id1, ver1)
	}

	byPath, err := repo.GetByPath(ctx, tenantID, path, nil, nil)
	if err != nil {
		t.Fatalf("get by path: %v", err)
	}
	if byPath.Content != "alpha beta gamma for search" {
		t.Fatalf("content: %q", byPath.Content)
	}

	entries, err := repo.Retrieve(ctx, model.RetrieveRequest{PathPrefix: path, Limit: 10}, tenantID, nil)
	if err != nil || len(entries) != 1 {
		t.Fatalf("retrieve default: err=%v n=%d", err, len(entries))
	}

	textEntries, err := repo.Retrieve(ctx, model.RetrieveRequest{PathPrefix: path, Limit: 10, Query: "alpha"}, tenantID, nil)
	if err != nil {
		t.Fatalf("retrieve fts: %v", err)
	}
	if len(textEntries) < 1 {
		t.Fatal("expected FTS hit")
	}

	v2 := 2
	id2, ver2, sup, err := repo.Store(ctx, model.StoreRequest{Path: path, Content: "second revision"}, tenantID)
	if err != nil {
		t.Fatalf("store v2: %v", err)
	}
	if ver2 != 2 || sup == nil || *sup != id1 {
		t.Fatalf("supersede: id2=%d ver2=%d sup=%v", id2, ver2, sup)
	}
	_ = id2

	hist, err := repo.ListPathHistory(ctx, tenantID, path, 10)
	if err != nil || len(hist) < 2 {
		t.Fatalf("history: err=%v len=%d", err, len(hist))
	}

	prev, err := repo.GetHistoricalVersion(ctx, tenantID, path, &v2, nil)
	if err != nil {
		t.Fatalf("get historical by version: %v", err)
	}
	if prev.Version != 2 {
		t.Fatalf("version: %d", prev.Version)
	}

	expFull, err := repo.ExportMemories(ctx, tenantID, path, 5, true)
	if err != nil || len(expFull) < 1 {
		t.Fatalf("export emb: err=%v n=%d", err, len(expFull))
	}
	expNoEmb, err := repo.ExportMemories(ctx, tenantID, path, 5, false)
	if err != nil || len(expNoEmb) < 1 {
		t.Fatalf("export no emb: err=%v n=%d", err, len(expNoEmb))
	}

	nDel, err := repo.CompactPathHistory(ctx, tenantID, path, 5)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if nDel < 0 {
		t.Fatalf("deleted: %d", nDel)
	}

	cur, err := repo.GetByPath(ctx, tenantID, path, nil, nil)
	if err != nil {
		t.Fatalf("current after compact: %v", err)
	}
	distPath := path + ".distilled"
	var distillID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO distilled_knowledge (tenant_id, path, summary, source_entry_ids, version, confidence_score)
		VALUES ($1::uuid, $2::ltree, $3, $4::bigint[], 1, 0.9)
		RETURNING id`,
		tenantID, distPath, "integration distilled", []int64{cur.ID},
	).Scan(&distillID); err != nil {
		t.Fatalf("insert distilled: %v", err)
	}
	lin := NewLineageRepository(pool, pool)
	if _, err := lin.DistilledLineage(ctx, tenantID, distillID); err != nil {
		t.Fatalf("distilled lineage: %v", err)
	}

	statsRepo := NewStatsRepository(pool, pool)
	st, err := statsRepo.TenantStats(ctx, tenantID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.ActiveMemories+st.SupersededMemories < 1 {
		t.Fatalf("stats empty: %+v", st)
	}

	evRepo := NewEventRepository(pool)
	eid, ets, err := evRepo.Insert(ctx, tenantID, "integration.test", map[string]interface{}{"k": 1}, nil)
	if err != nil || eid == "" {
		t.Fatalf("event insert: err=%v id=%q", err, eid)
	}
	_ = ets

	links := NewLinksRepository(pool, pool)
	link, err := links.Create(ctx, tenantID, model.CreateLinkRequest{FromPath: path, ToPath: path + ".alias", LinkType: "related"})
	if err != nil {
		t.Fatalf("link create: %v", err)
	}
	ls, err := links.List(ctx, tenantID, path, "", "", 10)
	if err != nil || len(ls) < 1 {
		t.Fatalf("link list: err=%v n=%d", err, len(ls))
	}
	_ = link

	encID, _, _, err := repo.Store(ctx, model.StoreRequest{
		Path: path + ".secret", Content: "classified", EncryptContent: true,
	}, tenantID)
	if err != nil {
		t.Fatalf("encrypted store: %v", err)
	}
	_ = encID
	gotEnc, err := repo.GetByPath(ctx, tenantID, path+".secret", nil, nil)
	if err != nil || gotEnc.Content != "classified" {
		t.Fatalf("decrypt read: err=%v c=%q", err, gotEnc.Content)
	}

	emb := make([]float32, 1536)
	for i := range emb {
		emb[i] = 0.0001
	}
	vecPath := path + ".withvec"
	if _, _, _, err := repo.Store(ctx, model.StoreRequest{
		Path: vecPath, Content: "embedding sample text for hybrid retrieval",
		Embedding: emb, EmbeddingModel: "m", EmbeddingSpace: "default",
	}, tenantID); err != nil {
		t.Fatalf("store vec: %v", err)
	}
	if _, err := repo.Retrieve(ctx, model.RetrieveRequest{PathPrefix: path, Limit: 5}, tenantID, emb); err != nil {
		t.Fatalf("retrieve vec: %v", err)
	}
	if _, err := repo.Retrieve(ctx, model.RetrieveRequest{PathPrefix: path, Limit: 5, Query: "embedding"}, tenantID, emb); err != nil {
		t.Fatalf("retrieve hybrid: %v", err)
	}

	linRepo := NewLineageRepository(pool, pool)
	if _, err := linRepo.MemoryLineage(ctx, tenantID, path); err != nil {
		t.Fatalf("lineage: %v", err)
	}

	aud := NewAuditRepository(pool)
	if _, _, err := aud.List(ctx, tenantID, 10, 0, nil); err != nil {
		t.Fatalf("audit: %v", err)
	}
	since := time.Now().Add(-24 * time.Hour)
	if _, _, err := aud.List(ctx, tenantID, 5, 0, &since); err != nil {
		t.Fatalf("audit since: %v", err)
	}

	if _, err := repo.ListPathHistory(ctx, tenantID, path, 0); err != nil {
		t.Fatalf("history limit norm: %v", err)
	}
}

func TestIntegration_MemoryRepository_cursorPagination(t *testing.T) {
	ctx := context.Background()
	pool := testDBPool(t)
	tenantID := "00000000-0000-0000-0000-000000000000"
	setTenant(t, ctx, pool, tenantID)

	base := fmt.Sprintf("root.cursor.%d", time.Now().UnixNano())
	repo := NewMemoryRepository(pool, pool)

	for i := 0; i < 5; i++ {
		p := fmt.Sprintf("%s.row%d", base, i)
		if _, _, _, err := repo.Store(ctx, model.StoreRequest{
			Path: p, Content: fmt.Sprintf("content-%d", i),
		}, tenantID); err != nil {
			t.Fatalf("store %s: %v", p, err)
		}
		time.Sleep(3 * time.Millisecond)
	}

	page1, err := repo.Retrieve(ctx, model.RetrieveRequest{PathPrefix: base, Limit: 2}, tenantID, nil)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("page1: want limit+1=3 rows, got %d", len(page1))
	}

	cursor, err := model.EncodeCursor(model.Cursor{
		Version:       1,
		SortKey:       model.SortKeyCreatedAtIDDesc,
		LastID:        page1[1].ID,
		LastTimestamp: page1[1].CreatedAt,
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	page2, err := repo.Retrieve(ctx, model.RetrieveRequest{
		PathPrefix: base,
		Limit:      2,
		Cursor:     cursor,
	}, tenantID, nil)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) < 1 {
		t.Fatal("page2: expected at least one row")
	}
	if page2[0].ID == page1[0].ID {
		t.Fatalf("page2 overlaps page1: same id %d", page2[0].ID)
	}

	_, err = repo.Retrieve(ctx, model.RetrieveRequest{
		PathPrefix: base,
		Limit:      2,
		Cursor:     "not-valid-cursor",
	}, tenantID, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid cursor") {
		t.Fatalf("malformed cursor: err=%v", err)
	}

	_, err = repo.Retrieve(ctx, model.RetrieveRequest{
		PathPrefix: base,
		Limit:      2,
		Query:      "content",
		Cursor:     cursor,
	}, tenantID, nil)
	if err == nil || !strings.Contains(err.Error(), "cursor pagination requires") {
		t.Fatalf("cursor+query: err=%v", err)
	}
}
