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
	"google.golang.org/grpc/credentials/insecure"
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
func newBufconnMemoryClient(t *testing.T) (pcmiv1.MemoryServiceClient, func()) {
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

	return pcmiv1.NewMemoryServiceClient(conn), cleanup
}

func TestIntegrationBufconn_HealthReady(t *testing.T) {
	client, cleanup := newBufconnMemoryClient(t)
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
	client, cleanup := newBufconnMemoryClient(t)
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
