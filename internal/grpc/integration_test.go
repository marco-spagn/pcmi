//go:build integration

package grpcserver_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/metadata"

	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
)

// Integration tests against a real MemoryService:
//
//   - Live server: set GRPC_HOST (optional, default localhost:50051) and GRPC_TEST_API_KEY.
//   - In-process (no listening port): package grpcserver — integration_bufconn_test.go uses DATABASE_URL + miniredis only
//     (including StreamEvents over Redis).
//
// Run (live):   GRPC_HOST=localhost:50051 GRPC_TEST_API_KEY=testkey123 go test -tags=integration ./internal/grpc/...
// Run (all):    DATABASE_URL=... go test -tags=integration ./internal/grpc/...
func TestGRPCStoreRetrieveTagsIntegration(t *testing.T) {
	host := os.Getenv("GRPC_HOST")
	if host == "" {
		host = "localhost:50051"
	}
	key := os.Getenv("GRPC_TEST_API_KEY")
	if key == "" {
		t.Skip("GRPC_TEST_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pcmiv1.NewMemoryServiceClient(conn)
	md := metadata.Pairs("x-api-key", key)
	ctx = metadata.NewOutgoingContext(ctx, md)

	path := "root.ci.grpc.integration." + time.Now().Format("150405")
	_, err = client.Store(ctx, &pcmiv1.StoreRequest{
		Path:           path,
		Content:        "grpc-integration",
		MetadataJson:   `{"suite":"integration"}`,
		Tags:           []string{"grpc-integration"},
		EmbeddingModel: "unspecified",
		EmbeddingSpace: "default",
		ApiKey:         key,
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	resp, err := client.Retrieve(ctx, &pcmiv1.RetrieveRequest{
		PathPrefix: path,
		Tags:       []string{"grpc-integration"},
		TagsMatch:  "all",
		Limit:      5,
		ApiKey:     key,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if resp.GetTotal() < 1 {
		t.Fatalf("retrieve total=%d", resp.GetTotal())
	}
	var entry *pcmiv1.RetrieveEntry
	for _, e := range resp.GetEntries() {
		if e.GetPath() == path {
			entry = e
			break
		}
	}
	if entry == nil {
		t.Fatalf("path %q not in entries", path)
	}
	if entry.GetTenantId() == "" || entry.GetEmbeddingModel() != "unspecified" {
		t.Fatalf("tenant/model: %+v", entry)
	}
	if len(entry.GetTags()) != 1 || entry.GetTags()[0] != "grpc-integration" {
		t.Fatalf("tags: %+v", entry.GetTags())
	}
	if entry.GetMetadataJson() == "" || entry.GetValidFromRfc3339() == "" || entry.GetCreatedAtRfc3339() == "" {
		t.Fatalf("metadata/timestamps: %+v", entry)
	}
}

func TestGRPCBatchStoreIntegration(t *testing.T) {
	host := os.Getenv("GRPC_HOST")
	if host == "" {
		host = "localhost:50051"
	}
	key := os.Getenv("GRPC_TEST_API_KEY")
	if key == "" {
		t.Skip("GRPC_TEST_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pcmiv1.NewMemoryServiceClient(conn)
	path := "root.ci.grpc.batch." + time.Now().Format("150405")
	resp, err := client.BatchStore(ctx, &pcmiv1.BatchStoreRequest{
		ApiKey: key,
		Items: []*pcmiv1.BatchStoreItem{
			{Path: path + ".a", Content: "one", Tags: []string{"batch-int"}, EmbeddingModel: "unspecified"},
		},
	})
	if err != nil {
		t.Fatalf("batch store: %v", err)
	}
	if resp.GetTotal() != 1 || resp.GetResults()[0].GetStatus() != "stored" {
		t.Fatalf("%+v", resp)
	}
}

func TestGRPCStoreInvalidExpiresIntegration(t *testing.T) {
	host := os.Getenv("GRPC_HOST")
	if host == "" {
		host = "localhost:50051"
	}
	key := os.Getenv("GRPC_TEST_API_KEY")
	if key == "" {
		t.Skip("GRPC_TEST_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pcmiv1.NewMemoryServiceClient(conn)
	_, err = client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: "root.ci.bad", Content: "x",
		ExpiresAtRfc3339: "not-a-timestamp",
	})
	if err == nil {
		t.Fatal("expected InvalidArgument")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
}

// testEmbedding1536 returns a minimal non-zero vector matching DB VECTOR(1536).
func testEmbedding1536() []float32 {
	v := make([]float32, 1536)
	v[0] = 1
	return v
}

func TestGRPCStoreClientEmbeddingIntegration(t *testing.T) {
	host := os.Getenv("GRPC_HOST")
	if host == "" {
		host = "localhost:50051"
	}
	key := os.Getenv("GRPC_TEST_API_KEY")
	dbURL := os.Getenv("DATABASE_URL")
	if key == "" || dbURL == "" {
		t.Skip("GRPC_TEST_API_KEY and DATABASE_URL required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pcmiv1.NewMemoryServiceClient(conn)
	path := "root.ci.grpc.embedding." + time.Now().Format("150405")
	storeResp, err := client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "grpc-embedding-vector",
		Embedding: testEmbedding1536(), EmbeddingModel: "client-supplied",
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if storeResp.GetId() == 0 {
		t.Fatalf("store response: %+v", storeResp)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var hasEmbedding bool
	err = pool.QueryRow(ctx,
		`SELECT embedding IS NOT NULL FROM memory_entries WHERE id = $1`,
		storeResp.GetId(),
	).Scan(&hasEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEmbedding {
		t.Fatal("expected embedding persisted")
	}
}

func TestGRPCOperationalIntegration(t *testing.T) {
	host := os.Getenv("GRPC_HOST")
	if host == "" {
		host = "localhost:50051"
	}
	key := os.Getenv("GRPC_TEST_API_KEY")
	if key == "" {
		t.Skip("GRPC_TEST_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pcmiv1.NewMemoryServiceClient(conn)
	path := "root.ci.grpc.ops." + time.Now().Format("150405")
	_, err = client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "ops", Tags: []string{"ops"}, EmbeddingModel: "unspecified",
	})
	if err != nil {
		t.Fatalf("store: %v", err)
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

	st, err := client.GetStats(ctx, &pcmiv1.GetStatsRequest{ApiKey: key})
	if err != nil || st.GetActiveMemories() < 1 {
		t.Fatalf("stats: %v %+v", err, st)
	}

	schemas, err := client.ListEventSchemas(ctx, &pcmiv1.ListEventSchemasRequest{})
	if err != nil || schemas.GetTotal() < 1 {
		t.Fatalf("schemas: %v %+v", err, schemas)
	}

	hist, err := client.GetHistory(ctx, &pcmiv1.GetHistoryRequest{ApiKey: key, Path: path, Limit: 10})
	if err != nil || hist.GetTotal() < 1 {
		t.Fatalf("history: %v %+v", err, hist)
	}
}

func TestGRPCGetMemoryCompactIntegration(t *testing.T) {
	host := os.Getenv("GRPC_HOST")
	if host == "" {
		host = "localhost:50051"
	}
	key := os.Getenv("GRPC_TEST_API_KEY")
	if key == "" {
		t.Skip("GRPC_TEST_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	client := pcmiv1.NewMemoryServiceClient(conn)
	path := "root.ci.grpc.getcompact." + time.Now().Format("150405")
	_, err = client.Store(ctx, &pcmiv1.StoreRequest{
		ApiKey: key, Path: path, Content: "get-compact-test",
		Tags: []string{"grpc-get"}, EmbeddingModel: "unspecified",
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	got, err := client.GetMemory(ctx, &pcmiv1.GetMemoryRequest{ApiKey: key, Path: path})
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got.GetEntry().GetPath() != path || got.GetEntry().GetContent() != "get-compact-test" {
		t.Fatalf("entry: %+v", got.GetEntry())
	}

	comp, err := client.Compact(ctx, &pcmiv1.CompactRequest{
		ApiKey: key, Path: path, KeepSuperseded: 20,
	})
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if comp.GetPath() != path {
		t.Fatalf("compact: %+v", comp)
	}
}

// TestResolveTenantIntegration requires DATABASE_URL and a valid test API key hash in DB.
func TestResolveTenantIntegration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	key := os.Getenv("GRPC_TEST_API_KEY")
	if dbURL == "" || key == "" {
		t.Skip("DATABASE_URL and GRPC_TEST_API_KEY required")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tenantID, role, err := grpcserver.ResolveTenantForTest(context.Background(), pool, key)
	if err != nil {
		t.Fatal(err)
	}
	if tenantID == "" || role == "" {
		t.Fatalf("tenant=%q role=%q", tenantID, role)
	}
}
