package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPServer_HandshakeInitialize(t *testing.T) {
	srv := NewServer(&mockAPI{}, nil)
	raw, _ := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
	})
	resp := srv.Handle(context.Background(), mustReq(t, raw))
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	b, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(b), "pcmi-mcp") {
		t.Fatalf("result=%s", b)
	}
}

func TestMCPServer_ListTools_Returns5Tools(t *testing.T) {
	srv := NewServer(&mockAPI{}, nil)
	resp := srv.Handle(context.Background(), rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	})
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", resp.Result)
	}
	tools, ok := result["tools"].([]toolDef)
	if !ok {
		// encoded via map in handler — re-decode
		b, _ := json.Marshal(result)
		var decoded struct {
			Tools []toolDef `json:"tools"`
		}
		_ = json.Unmarshal(b, &decoded)
		tools = decoded.Tools
	}
	if len(tools) != 5 {
		t.Fatalf("got %d tools", len(tools))
	}
	names := map[string]bool{}
	for _, td := range allTools() {
		names[td.Name] = true
	}
	for _, td := range tools {
		if !names[td.Name] {
			t.Fatalf("unexpected tool %s", td.Name)
		}
	}
}

func TestMCPServer_CallTool_RoutesCorrectly(t *testing.T) {
	var got string
	api := &mockAPI{
		storeFn: func(_ context.Context, body map[string]any) (map[string]any, error) {
			got, _ = body["path"].(string)
			return map[string]any{"ok": true}, nil
		},
	}
	srv := NewServer(api, nil)
	params, _ := json.Marshal(toolCallParams{
		Name:      "pcmi_store",
		Arguments: json.RawMessage(`{"path":"root.x","content":"y"}`),
	})
	resp := srv.Handle(context.Background(), rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`3`),
		Method:  "tools/call",
		Params:  params,
	})
	if resp.Error != nil {
		t.Fatalf("rpc error: %+v", resp.Error)
	}
	if got != "root.x" {
		t.Fatalf("path=%q", got)
	}
}

func TestMCPServer_InvalidTool_ReturnsMethodNotFound(t *testing.T) {
	srv := NewServer(&mockAPI{}, nil)
	params, _ := json.Marshal(toolCallParams{Name: "no_such_tool", Arguments: json.RawMessage(`{}`)})
	resp := srv.Handle(context.Background(), rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`4`),
		Method:  "tools/call",
		Params:  params,
	})
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected -32601, got %+v", resp.Error)
	}
}

func TestMCPServer_RunInitializeSmoke(t *testing.T) {
	srv := NewServer(&mockAPI{}, nil)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}` + "\n"
	var out bytes.Buffer
	if err := srv.Run(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"result"`) {
		t.Fatalf("out=%s", out.String())
	}
}

func mustReq(t *testing.T, raw []byte) rpcRequest {
	t.Helper()
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	return req
}
