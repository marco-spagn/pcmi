package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"

	"github.com/marco-spagn/pcmi/internal/version"
)

const mcpProtocolVersion = "2024-11-05"

type Server struct {
	api pcmiAPI
	log *slog.Logger
}

func NewServer(api pcmiAPI, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{api: api, log: log}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return e.Message
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      map[string]string `json:"serverInfo"`
}

func (s *Server) Handle(ctx context.Context, req rpcRequest) *rpcResponse {
	if req.JSONRPC != "2.0" {
		return s.errResp(req.ID, -32600, "invalid jsonrpc version")
	}
	switch req.Method {
	case "initialize":
		return s.handleInitialize(ctx, req)
	case "notifications/initialized", "initialized":
		return nil
	case "tools/list":
		return s.okResp(req.ID, map[string]any{"tools": allTools()})
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "resources/list":
		return s.okResp(req.ID, map[string]any{"resources": allResources()})
	case "resources/read":
		return s.handleResourcesRead(ctx, req)
	case "ping":
		return s.okResp(req.ID, map[string]any{})
	default:
		return s.errResp(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) handleInitialize(_ context.Context, req rpcRequest) *rpcResponse {
	var p initializeParams
	_ = json.Unmarshal(req.Params, &p)
	return s.okResp(req.ID, initializeResult{
		ProtocolVersion: mcpProtocolVersion,
		Capabilities: map[string]any{
			"tools":     map[string]any{},
			"resources": map[string]any{},
		},
		ServerInfo: map[string]string{
			"name":    "pcmi-mcp",
			"version": version.Tag,
		},
	})
}

func (s *Server) handleToolsCall(ctx context.Context, req rpcRequest) *rpcResponse {
	var p toolCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.errResp(req.ID, -32602, "invalid params: "+err.Error())
	}
	result, err := s.callTool(ctx, p.Name, p.Arguments)
	if err != nil {
		if rpcErr, ok := err.(*rpcError); ok {
			return s.errResp(req.ID, rpcErr.Code, rpcErr.Message)
		}
		return s.errResp(req.ID, -32603, err.Error())
	}
	return s.okResp(req.ID, result)
}

func (s *Server) handleResourcesRead(ctx context.Context, req rpcRequest) *rpcResponse {
	p, err := parseResourceReadParams(req.Params)
	if err != nil {
		return s.errResp(req.ID, -32602, "invalid params: "+err.Error())
	}
	if strings.TrimSpace(p.URI) == "" {
		return s.errResp(req.ID, -32602, "uri is required")
	}
	result, err := s.readResource(ctx, p.URI)
	if err != nil {
		if rpcErr, ok := err.(*rpcError); ok {
			return s.errResp(req.ID, rpcErr.Code, rpcErr.Message)
		}
		return s.errResp(req.ID, -32603, err.Error())
	}
	return s.okResp(req.ID, result)
}

func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	// MCP messages are line-delimited JSON on stdio.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.log.Error("decode request", "err", err)
			continue
		}
		resp := s.Handle(ctx, req)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *Server) okResp(id json.RawMessage, result any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) errResp(id json.RawMessage, code int, msg string) *rpcResponse {
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	}
}

