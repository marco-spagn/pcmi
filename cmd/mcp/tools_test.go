package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockAPI struct {
	storeFn      func(ctx context.Context, body map[string]any) (map[string]any, error)
	retrieveFn   func(ctx context.Context, body map[string]any) (map[string]any, error)
	getHistoryFn func(ctx context.Context, path string, limit int) (map[string]any, error)
	listPathsFn  func(ctx context.Context, pathPrefix string, limit int) ([]string, error)
	createLinkFn func(ctx context.Context, body map[string]any) (map[string]any, error)
	getMemoryFn  func(ctx context.Context, path string) (map[string]any, error)
	getStatsFn   func(ctx context.Context) (map[string]any, error)
}

func (m *mockAPI) Store(ctx context.Context, body map[string]any) (map[string]any, error) {
	if m.storeFn != nil {
		return m.storeFn(ctx, body)
	}
	return nil, nil
}
func (m *mockAPI) Retrieve(ctx context.Context, body map[string]any) (map[string]any, error) {
	if m.retrieveFn != nil {
		return m.retrieveFn(ctx, body)
	}
	return nil, nil
}
func (m *mockAPI) GetHistory(ctx context.Context, path string, limit int) (map[string]any, error) {
	if m.getHistoryFn != nil {
		return m.getHistoryFn(ctx, path, limit)
	}
	return nil, nil
}
func (m *mockAPI) ListPaths(ctx context.Context, pathPrefix string, limit int) ([]string, error) {
	if m.listPathsFn != nil {
		return m.listPathsFn(ctx, pathPrefix, limit)
	}
	return nil, nil
}
func (m *mockAPI) CreateLink(ctx context.Context, body map[string]any) (map[string]any, error) {
	if m.createLinkFn != nil {
		return m.createLinkFn(ctx, body)
	}
	return nil, nil
}
func (m *mockAPI) GetMemory(ctx context.Context, path string) (map[string]any, error) {
	if m.getMemoryFn != nil {
		return m.getMemoryFn(ctx, path)
	}
	return nil, nil
}
func (m *mockAPI) GetStats(ctx context.Context) (map[string]any, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx)
	}
	return nil, nil
}

func TestPCMIStoreTool_ValidInput_CallsAPI(t *testing.T) {
	var called bool
	api := &mockAPI{
		storeFn: func(_ context.Context, body map[string]any) (map[string]any, error) {
			called = true
			if body["path"] != "root.demo" {
				t.Fatalf("path=%v", body["path"])
			}
			return map[string]any{"status": "stored", "version": float64(1)}, nil
		},
	}
	srv := NewServer(api, nil)
	args, _ := json.Marshal(map[string]any{"path": "root.demo", "content": "hi"})
	res, err := srv.callTool(context.Background(), "pcmi_store", args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || !called {
		t.Fatalf("called=%v isError=%v text=%s", called, res.IsError, res.Content[0].Text)
	}
}

func TestPCMIStoreTool_MissingPath_ReturnsError(t *testing.T) {
	srv := NewServer(&mockAPI{}, nil)
	args, _ := json.Marshal(map[string]any{"content": "hi"})
	res, err := srv.callTool(context.Background(), "pcmi_store", args)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error, got %q", res.Content[0].Text)
	}
}

func TestPCMIRetrieveTool_ReturnsFormattedResults(t *testing.T) {
	api := &mockAPI{
		retrieveFn: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{
				"total": float64(1),
				"entries": []any{
					map[string]any{
						"path":            "root.a",
						"content":         "hello world",
						"version":         float64(2),
						"relevance_score": 0.9,
					},
				},
			}, nil
		},
	}
	srv := NewServer(api, nil)
	args, _ := json.Marshal(map[string]any{"path_prefix": "root", "limit": 5})
	res, err := srv.callTool(context.Background(), "pcmi_retrieve", args)
	if err != nil || res.IsError {
		t.Fatalf("err=%v isError=%v", err, res.IsError)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "root.a") || !strings.Contains(text, "Found 1 memories") {
		t.Fatalf("unexpected text: %s", text)
	}
}

func TestPCMIRetrieveTool_EmptyResults_GracefulMessage(t *testing.T) {
	api := &mockAPI{
		retrieveFn: func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"total": float64(0), "entries": []any{}}, nil
		},
	}
	srv := NewServer(api, nil)
	args, _ := json.Marshal(map[string]any{"path_prefix": "root.empty"})
	res, err := srv.callTool(context.Background(), "pcmi_retrieve", args)
	if err != nil || res.IsError {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(res.Content[0].Text, "No memories found") {
		t.Fatalf("text=%q", res.Content[0].Text)
	}
}

func TestHTTPPCMIClient_StoreAgainstMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/memories" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "secret" {
			t.Fatalf("api key missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "stored"})
	}))
	defer ts.Close()

	c := newHTTPPCMIClient(ts.URL, "secret")
	out, err := c.Store(context.Background(), map[string]any{"path": "p", "content": "c"})
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "stored" {
		t.Fatalf("out=%v", out)
	}
}
