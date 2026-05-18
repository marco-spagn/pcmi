package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestOpenAIProvider_HTTPHappyPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body := openai.EmbeddingResponse{
			Object: "list",
			Data: []openai.Embedding{
				{Embedding: []float32{-0.1, 0.2, 0.3}, Index: 0},
			},
			Model: "text-embedding-3-small",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encode: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := openai.DefaultConfig("sk-test")
	cfg.BaseURL = srv.URL + "/v1"
	p := NewOpenAIProviderWithConfig(cfg, "text-embedding-3-small")

	vec, err := p.Generate(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("len(vec)=%d", len(vec))
	}
	if vec[0] != -0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Fatalf("vec=%v", vec)
	}
}

func TestOpenAIProvider_HTTPEmptyData(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openai.EmbeddingResponse{Data: []openai.Embedding{}})
	}))
	t.Cleanup(srv.Close)

	cfg := openai.DefaultConfig("sk-test")
	cfg.BaseURL = srv.URL + "/v1"
	p := NewOpenAIProviderWithConfig(cfg, "m")

	if _, err := p.Generate(context.Background(), "x"); err == nil {
		t.Fatal("expected error for empty embeddings data")
	}
}

func TestOpenAIProvider_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	cfg := openai.DefaultConfig("sk-test")
	cfg.BaseURL = srv.URL + "/v1"
	p := NewOpenAIProviderWithConfig(cfg, "m")

	if _, err := p.Generate(context.Background(), "x"); err == nil {
		t.Fatal("expected error from 503")
	}
}
