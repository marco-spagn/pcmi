//go:build integration

package grpcserver

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/marco-spagn/pcmi/internal/event"
	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

const bufconnListenerSize = 2 << 20

func testIntegrationAPIKey(t *testing.T) string {
	t.Helper()
	if k := os.Getenv("GRPC_TEST_API_KEY"); k != "" {
		return k
	}
	return "testkey123"
}

// newBufconnMemoryClient starts an in-process gRPC server (same wiring as Start) over bufconn.
// Requires DATABASE_URL; Redis via miniredis. Does not need GRPC_HOST.
// The returned pool is the same handle used by the server (for direct SQL in tests).
func newBufconnMemoryClient(t *testing.T) (pcmiv1.MemoryServiceClient, *pgxpool.Pool, func()) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	pool, err := pgxpool.New(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("db: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	event.InitRedis(mr.Addr())

	read := pool
	memRepo := repository.NewMemoryRepository(pool, read)
	memSvc := service.NewMemoryService(memRepo, nil)

	lis := bufconn.Listen(bufconnListenerSize)
	srv := grpc.NewServer()
	pcmiv1.RegisterMemoryServiceServer(srv, &memoryServer{
		svc:         memSvc,
		db:          pool,
		readDB:      read,
		eventSvc:    service.NewEventService(repository.NewEventRepository(pool)),
		linksRepo:   repository.NewLinksRepository(pool, read),
		statsRepo:   repository.NewStatsRepository(pool, read),
		lineageRepo: repository.NewLineageRepository(pool, read),
		auditRepo:   repository.NewAuditRepository(pool),
		summarize:   service.NewSummarizeService(memRepo),
		memRepo:     memRepo,
	})

	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		srv.Stop()
		mr.Close()
		pool.Close()
		t.Fatalf("grpc client: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		srv.Stop()
		mr.Close()
		pool.Close()
	}

	return pcmiv1.NewMemoryServiceClient(conn), pool, cleanup
}

func TestIntegrationBufconn_HealthReady(t *testing.T) {
	client, _, cleanup := newBufconnMemoryClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	h, err := client.Health(ctx, &pcmiv1.HealthRequest{})
	if err != nil || h.GetStatus() != "ok" {
		t.Fatalf("health: %v %+v", err, h)
	}

	rd, err := client.Ready(ctx, &pcmiv1.ReadyRequest{})
	if err != nil {
		t.Fatalf("ready: %v", err)
	}
	if rd.GetStatus() != "ready" || !rd.GetDatabaseOk() || !rd.GetRedisOk() {
		t.Fatalf("ready: %+v", rd)
	}
}

func TestIntegrationBufconn_MemoryAndOperations(t *testing.T) {
	client, _, cleanup := newBufconnMemoryClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()

	key := testIntegrationAPIKey(t)
	path := "root.ci.bufconn." + time.Now().Format("150405")

	storeResp, err := client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "bufconn-body",
		Tags: []string{"bufconn"}, MetadataJson: `{"suite":"bufconn"}`,
		EmbeddingModel: "unspecified", EmbeddingSpace: "default",
	})
	if err != nil || storeResp.GetId() == 0 {
		t.Fatalf("store: %v %+v", err, storeResp)
	}

	ret, err := client.Retrieve(ctx, &pcmiv1.RetrieveRequest{
		ApiKey: key, PathPrefix: path, Limit: 5, Tags: []string{"bufconn"}, TagsMatch: "all",
	})
	if err != nil || ret.GetTotal() < 1 {
		t.Fatalf("retrieve: %v %+v", err, ret)
	}

	stream, err := client.RetrieveStream(ctx, &pcmiv1.RetrieveRequest{ApiKey: key, PathPrefix: path, Limit: 5})
	if err != nil {
		t.Fatalf("retrieve stream open: %v", err)
	}
	msg1, err := stream.Recv()
	if err != nil || msg1.GetHeader() == nil {
		t.Fatalf("retrieve stream header: %v %+v", err, msg1)
	}
	if msg1.GetHeader().GetTotal() < 1 {
		t.Fatalf("stream total: %+v", msg1.GetHeader())
	}
	msg2, err := stream.Recv()
	if err != nil || msg2.GetEntry() == nil || msg2.GetEntry().GetPath() != path {
		t.Fatalf("retrieve stream entry: %v %+v", err, msg2)
	}

	batch, err := client.BatchStore(ctx, &pcmiv1.BatchStoreRequest{
		ApiKey: key,
		Items: []*pcmiv1.BatchStoreItem{
			{Path: path + ".b", Content: "batch-b", EmbeddingModel: "unspecified"},
		},
	})
	if err != nil || batch.GetTotal() != 1 {
		t.Fatalf("batch store: %v %+v", err, batch)
	}

	bret, err := client.BatchRetrieve(ctx, &pcmiv1.BatchRetrieveRequest{
		ApiKey: key,
		Queries: []*pcmiv1.BatchRetrieveQuery{
			{PathPrefix: path, Limit: 10},
		},
	})
	if err != nil || bret.GetTotal() != 1 || len(bret.GetResults()) != 1 {
		t.Fatalf("batch retrieve: %v %+v", err, bret)
	}

	got, err := client.GetMemory(ctx, &pcmiv1.GetMemoryRequest{ApiKey: key, Path: path})
	if err != nil || got.GetEntry().GetContent() != "bufconn-body" {
		t.Fatalf("get memory: %v %+v", err, got)
	}

	ref, err := client.Refine(ctx, &pcmiv1.RefineRequest{ApiKey: key, PathPrefix: path})
	if err != nil || ref.GetStatus() != "queued" {
		t.Fatalf("refine: %v %+v", err, ref)
	}

	_, err = client.CreateLink(ctx, &pcmiv1.CreateLinkRequest{
		ApiKey: key, FromPath: path, ToPath: path + ".linked", LinkType: "related",
	})
	if err != nil {
		t.Fatalf("create link: %v", err)
	}

	links, err := client.ListLinks(ctx, &pcmiv1.ListLinksRequest{ApiKey: key, FromPath: path, Limit: 20})
	if err != nil || links.GetTotal() < 1 {
		t.Fatalf("list links: %v %+v", err, links)
	}

	st, err := client.GetStats(ctx, &pcmiv1.GetStatsRequest{ApiKey: key})
	if err != nil || st.GetActiveMemories() < 1 {
		t.Fatalf("stats: %v %+v", err, st)
	}

	schemas, err := client.ListEventSchemas(ctx, &pcmiv1.ListEventSchemasRequest{})
	if err != nil || schemas.GetTotal() < 1 {
		t.Fatalf("schemas: %v %+v", err, schemas)
	}

	_, err = client.IngestEvent(ctx, &pcmiv1.IngestEventRequest{
		ApiKey: key, EventType: "integration.bufconn", PayloadJson: `{"k":1}`,
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	hist, err := client.GetHistory(ctx, &pcmiv1.GetHistoryRequest{ApiKey: key, Path: path, Limit: 10})
	if err != nil || hist.GetTotal() < 1 {
		t.Fatalf("history: %v %+v", err, hist)
	}

	lineage, err := client.GetMemoryLineage(ctx, &pcmiv1.GetMemoryLineageRequest{ApiKey: key, Path: path})
	if err != nil || lineage.GetJson() == "" {
		t.Fatalf("lineage: %v %q", err, lineage.GetJson())
	}

	sum, err := client.Summarize(ctx, &pcmiv1.SummarizeRequest{ApiKey: key, PathPrefix: path, Limit: 5})
	if err != nil || sum.GetSummary() == "" {
		t.Fatalf("summarize: %v %+v", err, sum)
	}

	audit, err := client.ListAudit(ctx, &pcmiv1.ListAuditRequest{ApiKey: key, Limit: 5})
	if err != nil || audit.GetJson() == "" {
		t.Fatalf("audit: %v", err)
	}

	exp, err := client.ExportMemories(ctx, &pcmiv1.ExportMemoriesRequest{
		ApiKey: key, PathPrefix: path, Limit: 20, IncludeEmbeddings: false,
	})
	if err != nil || exp.GetExported() < 1 {
		t.Fatalf("export: %v %+v", err, exp)
	}

	wh, err := client.RegisterWebhook(ctx, &pcmiv1.RegisterWebhookRequest{
		ApiKey: key, Url: "https://example.invalid/grpc-hook", EventTypes: []string{"memory.stored"},
	})
	if err != nil || wh.GetId() == "" {
		t.Fatalf("register webhook: %v %+v", err, wh)
	}

	listWh, err := client.ListWebhooks(ctx, &pcmiv1.ListWebhooksRequest{ApiKey: key})
	if err != nil || listWh.GetTotal() < 1 {
		t.Fatalf("list webhooks: %v %+v", err, listWh)
	}

	dlq, err := client.ListWebhookDeadLetter(ctx, &pcmiv1.ListWebhookDeadLetterRequest{ApiKey: key, Limit: 10})
	if err != nil || dlq.GetJson() == "" {
		t.Fatalf("webhook dlq: %v", err)
	}

	dist, err := client.ListDistilled(ctx, &pcmiv1.ListDistilledRequest{ApiKey: key, PathPrefix: path, Limit: 5})
	if err != nil || dist.GetJson() == "" {
		t.Fatalf("list distilled: %v", err)
	}
	var distilledPayload map[string]any
	if err := json.Unmarshal([]byte(dist.GetJson()), &distilledPayload); err != nil {
		t.Fatalf("list distilled json: %v", err)
	}

	imp, err := client.ImportMemories(ctx, &pcmiv1.ImportMemoriesRequest{
		ApiKey: key, Mode: "skip",
		Items: []*pcmiv1.BatchStoreItem{
			{Path: path + ".imported", Content: "imported", EmbeddingModel: "unspecified"},
		},
	})
	if err != nil || imp.GetImported() < 1 {
		t.Fatalf("import: %v %+v", err, imp)
	}

	comp, err := client.Compact(ctx, &pcmiv1.CompactRequest{ApiKey: key, Path: path, KeepSuperseded: 20})
	if err != nil || comp.GetPath() != path {
		t.Fatalf("compact: %v %+v", err, comp)
	}

	sctx, scancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer scancel()
	evStream, err := client.StreamEvents(sctx, &pcmiv1.StreamEventsRequest{ApiKey: key})
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	for {
		_, err := evStream.Recv()
		if err != nil {
			break
		}
	}
}

func TestIntegrationBufconn_RollbackToVersion(t *testing.T) {
	client, _, cleanup := newBufconnMemoryClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	key := testIntegrationAPIKey(t)
	path := "root.ci.bufconn.rollback." + time.Now().Format("150405")

	if _, err := client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "rollback-v1", EmbeddingModel: "unspecified",
	}); err != nil {
		t.Fatalf("store v1: %v", err)
	}
	if _, err := client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "rollback-v2", EmbeddingModel: "unspecified",
	}); err != nil {
		t.Fatalf("store v2: %v", err)
	}

	got2, err := client.GetMemory(ctx, &pcmiv1.GetMemoryRequest{ApiKey: key, Path: path})
	if err != nil || got2.GetEntry().GetContent() != "rollback-v2" {
		t.Fatalf("before rollback: %v %+v", err, got2)
	}

	rb, err := client.Rollback(ctx, &pcmiv1.RollbackRequest{ApiKey: key, Path: path, Version: 1})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rb.GetStatus() != "rolled_back" || rb.GetRestoredFromVersion() != 1 {
		t.Fatalf("rollback response: %+v", rb)
	}

	got1, err := client.GetMemory(ctx, &pcmiv1.GetMemoryRequest{ApiKey: key, Path: path})
	if err != nil || got1.GetEntry().GetContent() != "rollback-v1" {
		t.Fatalf("after rollback: %v %+v", err, got1)
	}
}

func TestIntegrationBufconn_RollbackNotFound(t *testing.T) {
	client, _, cleanup := newBufconnMemoryClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	key := testIntegrationAPIKey(t)
	path := "root.ci.bufconn.badrollback." + time.Now().Format("150405")

	if _, err := client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "x", EmbeddingModel: "unspecified",
	}); err != nil {
		t.Fatalf("store: %v", err)
	}

	_, err := client.Rollback(ctx, &pcmiv1.RollbackRequest{ApiKey: key, Path: path, Version: 99})
	if err == nil {
		t.Fatal("expected NotFound")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestIntegrationBufconn_MigrateEmbeddings(t *testing.T) {
	client, _, cleanup := newBufconnMemoryClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	key := testIntegrationAPIKey(t)
	path := "root.ci.bufconn.migrate." + time.Now().Format("150405")

	if _, err := client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "emb-migrate", EmbeddingModel: "unspecified", EmbeddingSpace: "default",
	}); err != nil {
		t.Fatalf("store: %v", err)
	}

	out, err := client.MigrateEmbeddings(ctx, &pcmiv1.MigrateEmbeddingsRequest{
		ApiKey: key, PathPrefix: path, TargetModel: "text-embedding-3-small", EmbeddingSpace: "default",
	})
	if err != nil {
		t.Fatalf("migrate embeddings: %v", err)
	}
	if out.GetStatus() != "queued" || out.GetPathPrefix() != path {
		t.Fatalf("migrate response: %+v", out)
	}
}

func TestIntegrationBufconn_DistilledLineage(t *testing.T) {
	client, pool, cleanup := newBufconnMemoryClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	key := testIntegrationAPIKey(t)
	tenantID, _, err := ResolveTenantForTest(ctx, pool, key)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}

	path := "root.ci.bufconn.distill." + time.Now().Format("150405")
	storeResp, err := client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "distill-source", EmbeddingModel: "unspecified",
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	distPath := path + ".distilled"
	var distillID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO distilled_knowledge (tenant_id, path, summary, insights, confidence_score, source_entry_ids, version)
		VALUES ($1::uuid, $2::ltree, $3, '[]'::jsonb, 0.85, $4::bigint[], 1)
		RETURNING id`,
		tenantID, distPath, "integration bufconn distilled", []int64{storeResp.GetId()},
	).Scan(&distillID)
	if err != nil {
		t.Fatalf("insert distilled: %v", err)
	}

	lineage, err := client.GetDistilledLineage(ctx, &pcmiv1.GetDistilledLineageRequest{
		ApiKey: key, DistilledId: distillID,
	})
	if err != nil || lineage.GetJson() == "" {
		t.Fatalf("get distilled lineage: %v %q", err, lineage.GetJson())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(lineage.GetJson()), &payload); err != nil {
		t.Fatalf("lineage json: %v", err)
	}

	_, err = client.GetDistilledLineage(ctx, &pcmiv1.GetDistilledLineageRequest{ApiKey: key, DistilledId: 999999999999})
	if err == nil {
		t.Fatal("expected error for missing distilled id")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("distilled lineage missing: %v", err)
	}
}

func TestIntegrationBufconn_StoreRequiresAPIKey(t *testing.T) {
	client, _, cleanup := newBufconnMemoryClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := client.Store(ctx, &pcmiv1.StoreRequest{
		Path: "root.ci.bufconn.nokey", Content: "x", EmbeddingModel: "unspecified",
	})
	if err == nil {
		t.Fatal("expected Unauthenticated")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unauthenticated {
		t.Fatalf("got %v", err)
	}
}
