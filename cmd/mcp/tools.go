package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func allTools() []toolDef {
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{
			"type":       "object",
			"properties": props,
			"required":   required,
		}
	}
	return []toolDef{
		{
			Name:        "pcmi_store",
			Description: "Store a versioned memory at path (POST /v1/memories).",
			InputSchema: obj(map[string]any{
				"path":    map[string]any{"type": "string", "description": "ltree path, e.g. root.agent.note"},
				"content": map[string]any{"type": "string"},
				"metadata": map[string]any{"type": "object"},
				"tags":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			}, "path", "content"),
		},
		{
			Name:        "pcmi_retrieve",
			Description: "Hybrid retrieve under path_prefix (POST /v1/retrieve).",
			InputSchema: obj(map[string]any{
				"path_prefix": map[string]any{"type": "string"},
				"query":       map[string]any{"type": "string"},
				"limit":       map[string]any{"type": "integer", "default": 10},
			}, "path_prefix"),
		},
		{
			Name:        "pcmi_get_history",
			Description: "List all versions for a path (GET /v1/memories/history).",
			InputSchema: obj(map[string]any{
				"path":  map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer", "default": 50},
			}, "path"),
		},
		{
			Name:        "pcmi_list_paths",
			Description: "List distinct memory paths under path_prefix (export API).",
			InputSchema: obj(map[string]any{
				"path_prefix": map[string]any{"type": "string"},
				"limit":       map[string]any{"type": "integer", "default": 500},
			}, "path_prefix"),
		},
		{
			Name:        "pcmi_create_link",
			Description: "Create a directed link between two memory paths.",
			InputSchema: obj(map[string]any{
				"from_path": map[string]any{"type": "string"},
				"to_path":   map[string]any{"type": "string"},
				"link_type": map[string]any{"type": "string", "default": "related"},
				"metadata":  map[string]any{"type": "object"},
			}, "from_path", "to_path"),
		},
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Server) callTool(ctx context.Context, name string, args json.RawMessage) (toolResult, error) {
	switch name {
	case "pcmi_store":
		return s.toolStore(ctx, args)
	case "pcmi_retrieve":
		return s.toolRetrieve(ctx, args)
	case "pcmi_get_history":
		return s.toolGetHistory(ctx, args)
	case "pcmi_list_paths":
		return s.toolListPaths(ctx, args)
	case "pcmi_create_link":
		return s.toolCreateLink(ctx, args)
	default:
		return toolResult{}, &rpcError{Code: -32601, Message: "tool not found: " + name}
	}
}

func (s *Server) toolStore(ctx context.Context, args json.RawMessage) (toolResult, error) {
	var in struct {
		Path     string         `json:"path"`
		Content  string         `json:"content"`
		Metadata map[string]any `json:"metadata"`
		Tags     []string       `json:"tags"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Path) == "" {
		return toolError("path is required"), nil
	}
	body := map[string]any{
		"path":            in.Path,
		"content":         in.Content,
		"embedding_model": "unspecified",
	}
	if in.Metadata != nil {
		body["metadata"] = in.Metadata
	}
	if len(in.Tags) > 0 {
		body["tags"] = in.Tags
	}
	resp, err := s.api.Store(ctx, body)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return textResult(formatJSON(resp)), nil
}

func (s *Server) toolRetrieve(ctx context.Context, args json.RawMessage) (toolResult, error) {
	var in struct {
		PathPrefix string `json:"path_prefix"`
		Query      string `json:"query"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(in.PathPrefix) == "" {
		return toolError("path_prefix is required"), nil
	}
	if in.Limit <= 0 {
		in.Limit = 10
	}
	resp, err := s.api.Retrieve(ctx, map[string]any{
		"path_prefix": in.PathPrefix,
		"query":       in.Query,
		"limit":       in.Limit,
	})
	if err != nil {
		return toolError(err.Error()), nil
	}
	return textResult(formatRetrieve(resp)), nil
}

func (s *Server) toolGetHistory(ctx context.Context, args json.RawMessage) (toolResult, error) {
	var in struct {
		Path  string `json:"path"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Path) == "" {
		return toolError("path is required"), nil
	}
	if in.Limit <= 0 {
		in.Limit = 50
	}
	resp, err := s.api.GetHistory(ctx, in.Path, in.Limit)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return textResult(formatJSON(resp)), nil
}

func (s *Server) toolListPaths(ctx context.Context, args json.RawMessage) (toolResult, error) {
	var in struct {
		PathPrefix string `json:"path_prefix"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(in.PathPrefix) == "" {
		return toolError("path_prefix is required"), nil
	}
	paths, err := s.api.ListPaths(ctx, in.PathPrefix, in.Limit)
	if err != nil {
		return toolError(err.Error()), nil
	}
	out := map[string]any{"path_prefix": in.PathPrefix, "paths": paths, "total": len(paths)}
	return textResult(formatJSON(out)), nil
}

func (s *Server) toolCreateLink(ctx context.Context, args json.RawMessage) (toolResult, error) {
	var in struct {
		FromPath string         `json:"from_path"`
		ToPath   string         `json:"to_path"`
		LinkType string         `json:"link_type"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return toolError("invalid arguments: " + err.Error()), nil
	}
	if strings.TrimSpace(in.FromPath) == "" || strings.TrimSpace(in.ToPath) == "" {
		return toolError("from_path and to_path are required"), nil
	}
	linkType := strings.TrimSpace(in.LinkType)
	if linkType == "" {
		linkType = "related"
	}
	body := map[string]any{
		"from_path": in.FromPath,
		"to_path":   in.ToPath,
		"link_type": linkType,
		"metadata":  map[string]any{},
	}
	if in.Metadata != nil {
		body["metadata"] = in.Metadata
	}
	resp, err := s.api.CreateLink(ctx, body)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return textResult(formatJSON(resp)), nil
}

func formatRetrieve(resp map[string]any) string {
	entries, _ := resp["entries"].([]any)
	total, _ := resp["total"].(float64)
	if len(entries) == 0 {
		return fmt.Sprintf("No memories found (total=%d).", int(total))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d memories:\n", int(total))
	for i, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["path"].(string)
		content, _ := m["content"].(string)
		version, _ := m["version"].(float64)
		score, _ := m["relevance_score"].(float64)
		fmt.Fprintf(&b, "\n%d. %s (v%.0f", i+1, path, version)
		if score > 0 {
			fmt.Fprintf(&b, ", score=%.3f", score)
		}
		b.WriteString(")\n")
		if len(content) > 200 {
			content = content[:200] + "…"
		}
		b.WriteString(content)
		b.WriteByte('\n')
	}
	if nc, _ := resp["next_cursor"].(string); nc != "" {
		fmt.Fprintf(&b, "\n(next page: cursor=%q)\n", nc)
	}
	return b.String()
}

func formatJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func textResult(text string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}

func toolError(msg string) toolResult {
	return toolResult{
		Content: []toolContent{{Type: "text", Text: msg}},
		IsError: true,
	}
}
