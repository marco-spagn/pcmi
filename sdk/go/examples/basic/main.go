// Basic PCMI Go SDK smoke against a running API.
//
//	export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
//	go run ./examples/basic
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/marco-spagn/pcmi/sdk/go/pcmi"
)

func main() {
	base := envOr("PCMI_BASE_URL", "http://localhost:8000")
	key := envOr("PCMI_API_KEY", "testkey123")
	path := "root.sdk.go.smoke"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := pcmi.NewClient(base, key)
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.Store(ctx, path, "hello from go sdk", nil, &pcmi.StoreOptions{
		Tags:           []string{"sdk-smoke"},
		EmbeddingModel: "unspecified",
	})
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	out, err := client.Retrieve(ctx, path, "", 5, &pcmi.RetrieveOptions{
		Tags:      []string{"sdk-smoke"},
		TagsMatch: "all",
	})
	if err != nil {
		log.Fatalf("retrieve: %v", err)
	}
	fmt.Println("retrieve total:", out.Total)

	compact, err := client.Compact(ctx, path, 20)
	if err != nil {
		log.Fatalf("compact: %v", err)
	}
	fmt.Println("compact:", compact)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
