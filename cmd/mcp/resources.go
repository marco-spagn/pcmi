package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const (
	resourceMemoryPrefix = "pcmi://memory/"
	resourceStatsURI     = "pcmi://stats"
)

type resourceDef struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

func allResources() []resourceDef {
	return []resourceDef{
		{
			URI:         resourceMemoryPrefix + "{path}",
			Name:        "memory",
			Description: "Current memory entry at an ltree path",
			MimeType:    "application/json",
		},
		{
			URI:         resourceStatsURI,
			Name:        "stats",
			Description: "Tenant memory statistics",
			MimeType:    "application/json",
		},
	}
}

type resourceReadParams struct {
	URI string `json:"uri"`
}

type resourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type resourceReadResult struct {
	Contents []resourceContents `json:"contents"`
}

func (s *Server) readResource(ctx context.Context, uri string) (resourceReadResult, error) {
	switch {
	case uri == resourceStatsURI:
		stats, err := s.api.GetStats(ctx)
		if err != nil {
			return resourceReadResult{}, err
		}
		return resourceReadResult{Contents: []resourceContents{{
			URI:      uri,
			MimeType: "application/json",
			Text:     formatJSON(stats),
		}}}, nil
	case strings.HasPrefix(uri, resourceMemoryPrefix):
		rawPath := strings.TrimPrefix(uri, resourceMemoryPrefix)
		path, err := url.PathUnescape(rawPath)
		if err != nil {
			path = rawPath
		}
		path = strings.TrimSpace(path)
		if path == "" {
			return resourceReadResult{}, fmt.Errorf("memory path is required in uri")
		}
		entry, err := s.api.GetMemory(ctx, path)
		if err != nil {
			return resourceReadResult{}, err
		}
		return resourceReadResult{Contents: []resourceContents{{
			URI:      uri,
			MimeType: "application/json",
			Text:     formatJSON(entry),
		}}}, nil
	default:
		return resourceReadResult{}, &rpcError{Code: -32602, Message: "unknown resource: " + uri}
	}
}

func parseResourceReadParams(raw json.RawMessage) (resourceReadParams, error) {
	var p resourceReadParams
	if len(raw) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	return p, nil
}
