package pcmi

import (
	"context"
	"net/url"
	"strconv"
)

// AgentSession is returned by session lifecycle APIs.
type AgentSession struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	AgentID   *string        `json:"agent_id"`
	Metadata  map[string]any `json:"metadata"`
	StartedAt string         `json:"started_at"`
	EndedAt   *string        `json:"ended_at"`
	Status    string         `json:"status"`
}

// SessionMemoriesResponse lists working-memory rows for a session.
type SessionMemoriesResponse struct {
	SessionID string        `json:"session_id"`
	Entries   []MemoryEntry `json:"entries"`
	Total     int           `json:"total"`
}

// CreateSession starts an agent session (POST /v1/sessions).
func (c *Client) CreateSession(ctx context.Context, agentID string, metadata map[string]any) (*AgentSession, error) {
	body := map[string]any{}
	if agentID != "" {
		body["agent_id"] = agentID
	}
	if metadata != nil {
		body["metadata"] = metadata
	}
	var out AgentSession
	if err := c.doJSON(ctx, "POST", "/v1/sessions", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EndSession ends a session (DELETE /v1/sessions/{id}).
func (c *Client) EndSession(ctx context.Context, sessionID string) (*AgentSession, error) {
	var out AgentSession
	path := "/v1/sessions/" + url.PathEscape(sessionID)
	if err := c.doJSON(ctx, "DELETE", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StoreSessionMemory stores working memory scoped to a session (POST /v1/sessions/{id}/memories).
func (c *Client) StoreSessionMemory(ctx context.Context, sessionID, path, content string, metadata map[string]any, tags []string) error {
	body := map[string]any{
		"path":    path,
		"content": content,
	}
	if metadata != nil {
		body["metadata"] = metadata
	}
	if len(tags) > 0 {
		body["tags"] = tags
	}
	return c.doJSON(ctx, "POST", "/v1/sessions/"+url.PathEscape(sessionID)+"/memories", body, nil)
}

// ListSessionMemories lists session working memories (GET /v1/sessions/{id}/memories).
func (c *Client) ListSessionMemories(ctx context.Context, sessionID string, limit int, pathPrefix string, includeLongTerm bool) (*SessionMemoriesResponse, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if pathPrefix != "" {
		q.Set("path_prefix", pathPrefix)
	}
	if includeLongTerm {
		q.Set("include_long_term", "true")
	}
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/memories"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out SessionMemoriesResponse
	if err := c.doJSON(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PromoteSession promotes working memory to long-term paths (POST /v1/sessions/{id}/promote).
func (c *Client) PromoteSession(ctx context.Context, sessionID, targetPrefix string) (map[string]any, error) {
	body := map[string]any{}
	if targetPrefix != "" {
		body["target_prefix"] = targetPrefix
	}
	var out map[string]any
	path := "/v1/sessions/" + url.PathEscape(sessionID) + "/promote"
	if err := c.doJSON(ctx, "POST", path, body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
