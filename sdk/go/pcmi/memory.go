package pcmi

import (
	"context"
	"net/url"
	"strconv"
)

// MemoryEntry is a row returned by retrieve/export APIs.
type MemoryEntry struct {
	ID              int64    `json:"id"`
	TenantID        string   `json:"tenant_id"`
	Path            string   `json:"path"`
	Content         string   `json:"content"`
	Metadata        map[string]any `json:"metadata"`
	Tags            []string `json:"tags"`
	EmbeddingModel  string   `json:"embedding_model"`
	EmbeddingSpace  string   `json:"embedding_space"`
	Version         int      `json:"version"`
	ValidFrom       string   `json:"valid_from"`
	ValidTo         *string  `json:"valid_to"`
	RelevanceScore  *float64 `json:"relevance_score"`
	Importance      *float64 `json:"importance"`
	AccessCount     int      `json:"access_count"`
	LastAccessedAt  *string  `json:"last_accessed_at"`
	CreatedAt       string   `json:"created_at"`
}

// StoreResponse is returned by POST /v1/memories.
type StoreResponse struct {
	ID      int64  `json:"id"`
	Path    string `json:"path"`
	Version int    `json:"version"`
}

// RetrieveResponse is returned by POST /v1/retrieve.
type RetrieveResponse struct {
	Entries []MemoryEntry `json:"entries"`
	Total   int           `json:"total"`
}

// Store writes a memory at path (POST /v1/memories).
func (c *Client) Store(ctx context.Context, path, content string, metadata map[string]any, opts *StoreOptions) (*StoreResponse, error) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	body := map[string]any{
		"path":     path,
		"content":  content,
		"metadata": metadata,
	}
	applyStoreOpts(body, opts)
	var out StoreResponse
	if err := c.doJSON(ctx, "POST", "/v1/memories", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Retrieve searches memories under pathPrefix (POST /v1/retrieve).
func (c *Client) Retrieve(ctx context.Context, pathPrefix, query string, limit int, opts *RetrieveOptions) (*RetrieveResponse, error) {
	body := map[string]any{
		"path_prefix": pathPrefix,
		"query":       query,
		"limit":       limit,
	}
	applyRetrieveOpts(body, opts)
	var out RetrieveResponse
	if err := c.doJSON(ctx, "POST", "/v1/retrieve", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMemory fetches a single memory (GET /v1/memories/{path}).
func (c *Client) GetMemory(ctx context.Context, path string, version *int, asOf string) (*MemoryEntry, error) {
	q := url.Values{}
	if version != nil {
		q.Set("version", strconv.Itoa(*version))
	}
	if asOf != "" {
		q.Set("as_of", asOf)
	}
	pathSuffix := "/v1/memories/" + url.PathEscape(path)
	if enc := q.Encode(); enc != "" {
		pathSuffix += "?" + enc
	}
	var out MemoryEntry
	if err := c.doJSON(ctx, "GET", pathSuffix, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Rollback restores a prior version (POST /v1/memories/rollback).
func (c *Client) Rollback(ctx context.Context, path string, version *int, asOf string) (map[string]any, error) {
	body := map[string]any{"path": path}
	if version != nil {
		body["version"] = *version
	}
	if asOf != "" {
		body["as_of"] = asOf
	}
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/memories/rollback", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// BatchStore stores multiple memories (POST /v1/memories/batch).
func (c *Client) BatchStore(ctx context.Context, items []map[string]any) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/memories/batch", map[string]any{"items": items}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// BatchRetrieve runs multiple retrieve queries (POST /v1/retrieve/batch).
func (c *Client) BatchRetrieve(ctx context.Context, queries []map[string]any) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/retrieve/batch", map[string]any{"queries": queries}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Compact compacts version history for a path (POST /v1/memories/compact).
func (c *Client) Compact(ctx context.Context, path string, keepSuperseded int) (map[string]any, error) {
	body := map[string]any{"path": path, "keep_superseded": keepSuperseded}
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/memories/compact", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Refine queues distillation for a path prefix (POST /v1/memories/refine).
func (c *Client) Refine(ctx context.Context, pathPrefix string) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "POST", "/v1/memories/refine", map[string]any{"path_prefix": pathPrefix}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetHistory lists versions for a path (GET /v1/memories/history).
func (c *Client) GetHistory(ctx context.Context, path string, limit int) (map[string]any, error) {
	q := url.Values{"path": {path}}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/memories/history?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TenantStats returns tenant usage stats (GET /v1/stats).
func (c *Client) TenantStats(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.doJSON(ctx, "GET", "/v1/stats", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
