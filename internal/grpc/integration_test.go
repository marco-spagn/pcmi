//go:build integration

package grpcserver_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
)

// Integration tests dial a running API with gRPC (see scripts/grpc_health_smoke.go).
// Run: GRPC_HOST=localhost:50051 GRPC_TEST_API_KEY=testkey123 go test -tags=integration ./internal/grpc/...
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
		Tags:           []string{"grpc-integration"},
		EmbeddingModel: "unspecified",
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
	found := false
	for _, e := range resp.GetEntries() {
		if e.GetPath() == path {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("path %q not in entries", path)
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
