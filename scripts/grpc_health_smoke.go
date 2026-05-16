//go:build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	host := os.Getenv("GRPC_HOST")
	if host == "" {
		host = "localhost:50051"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pcmiv1.NewMemoryServiceClient(conn)
	resp, err := client.Health(ctx, &pcmiv1.HealthRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "health: %v\n", err)
		os.Exit(1)
	}
	expected := os.Getenv("PCMI_EXPECT_VERSION")
	if expected == "" {
	expected = "v1.21.0"
	}
	if resp.GetStatus() != "ok" || resp.GetVersion() != expected {
		fmt.Fprintf(os.Stderr, "unexpected health: %+v (want version %s)\n", resp, expected)
		os.Exit(1)
	}
	ready, err := client.Ready(ctx, &pcmiv1.ReadyRequest{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ready: %v\n", err)
		os.Exit(1)
	}
	if ready.GetStatus() != "ready" || !ready.GetDatabaseOk() || !ready.GetRedisOk() || ready.GetVersion() != expected {
		fmt.Fprintf(os.Stderr, "unexpected ready: %+v (want status ready, deps ok, version %s)\n", ready, expected)
		os.Exit(1)
	}
	fmt.Println("gRPC health ok", resp.GetVersion(), "ready ok")

	key := strings.TrimSpace(os.Getenv("GRPC_TEST_API_KEY"))
	if key == "" {
		return
	}
	_, err = client.BatchStore(ctx, &pcmiv1.BatchStoreRequest{ApiKey: key})
	if err == nil {
		fmt.Fprintln(os.Stderr, "BatchStore with no items: expected error")
		os.Exit(1)
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		fmt.Fprintf(os.Stderr, "BatchStore empty: want InvalidArgument, got %v\n", err)
		os.Exit(1)
	}
	_, err = client.BatchRetrieve(ctx, &pcmiv1.BatchRetrieveRequest{ApiKey: key})
	if err == nil {
		fmt.Fprintln(os.Stderr, "BatchRetrieve with no queries: expected error")
		os.Exit(1)
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		fmt.Fprintf(os.Stderr, "BatchRetrieve empty: want InvalidArgument, got %v\n", err)
		os.Exit(1)
	}
	rstream, err := client.RetrieveStream(ctx, &pcmiv1.RetrieveRequest{
		ApiKey: key, PathPrefix: "__grpc_smoke_no_matches__", Limit: 3,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "RetrieveStream: %v\n", err)
		os.Exit(1)
	}
	first, err := rstream.Recv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "RetrieveStream first frame: %v\n", err)
		os.Exit(1)
	}
	if first.GetHeader() == nil {
		fmt.Fprintf(os.Stderr, "RetrieveStream: expected header first, got %+v\n", first)
		os.Exit(1)
	}
	for {
		_, err := rstream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "RetrieveStream recv: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("gRPC BatchStore + BatchRetrieve + RetrieveStream smoke ok")
}
