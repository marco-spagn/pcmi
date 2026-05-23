package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// pcmiAPI is the HTTP surface the MCP tools call.
type pcmiAPI interface {
	Store(ctx context.Context, body map[string]any) (map[string]any, error)
	Retrieve(ctx context.Context, body map[string]any) (map[string]any, error)
	GetHistory(ctx context.Context, path string, limit int) (map[string]any, error)
	ListPaths(ctx context.Context, pathPrefix string, limit int) ([]string, error)
	CreateLink(ctx context.Context, body map[string]any) (map[string]any, error)
	GetMemory(ctx context.Context, path string) (map[string]any, error)
	GetStats(ctx context.Context) (map[string]any, error)
}

type httpPCMIClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func newHTTPPCMIClient(baseURL, apiKey string) *httpPCMIClient {
	return &httpPCMIClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *httpPCMIClient) Store(ctx context.Context, body map[string]any) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPost, "/v1/memories", body)
}

func (c *httpPCMIClient) Retrieve(ctx context.Context, body map[string]any) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPost, "/v1/retrieve", body)
}

func (c *httpPCMIClient) GetHistory(ctx context.Context, path string, limit int) (map[string]any, error) {
	q := url.Values{"path": {path}}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	return c.doJSON(ctx, http.MethodGet, "/v1/memories/history?"+q.Encode(), nil)
}

func (c *httpPCMIClient) ListPaths(ctx context.Context, pathPrefix string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 500
	}
	body := map[string]any{
		"path_prefix": pathPrefix,
		"limit":       limit,
	}
	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/memories/export", body)
	if err != nil {
		return nil, err
	}
	entries, _ := resp["entries"].([]any)
	seen := make(map[string]struct{})
	var paths []string
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		p, _ := m["path"].(string)
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	return paths, nil
}

func (c *httpPCMIClient) CreateLink(ctx context.Context, body map[string]any) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodPost, "/v1/memories/links", body)
}

func (c *httpPCMIClient) GetMemory(ctx context.Context, path string) (map[string]any, error) {
	escaped := url.PathEscape(path)
	return c.doJSON(ctx, http.MethodGet, "/v1/memories/"+escaped, nil)
}

func (c *httpPCMIClient) GetStats(ctx context.Context) (map[string]any, error) {
	return c.doJSON(ctx, http.MethodGet, "/v1/stats", nil)
}

func (c *httpPCMIClient) doJSON(ctx context.Context, method, path string, body map[string]any) (map[string]any, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var errBody map[string]any
		_ = json.Unmarshal(data, &errBody)
		msg, _ := errBody["error"].(string)
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("pcmi api %s: %s", resp.Status, msg)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
