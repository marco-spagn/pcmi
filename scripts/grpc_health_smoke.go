//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	pcmiv1 "github.com/marco-spagn/pcmi/internal/grpc/pcmiv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
		expected = "v1.15.0"
	}
	if resp.GetStatus() != "ok" || resp.GetVersion() != expected {
		fmt.Fprintf(os.Stderr, "unexpected health: %+v (want version %s)\n", resp, expected)
		os.Exit(1)
	}
	fmt.Println("gRPC health ok", resp.GetVersion())
}
