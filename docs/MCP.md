# PCMI MCP server

The **pcmi-mcp** binary (`cmd/mcp`) exposes PCMI over the [Model Context Protocol](https://modelcontextprotocol.io/) on **stdio** using JSON-RPC 2.0. Agents connect without embedding HTTP details in prompts.

## Requirements

- Running PCMI API (`make infra-up` or deployed instance)
- API key with appropriate RBAC (read-only keys can use retrieve/history/list/stats; writes need `user` or `admin` role)

## Environment

| Variable | Example |
|----------|---------|
| `PCMI_BASE_URL` | `http://localhost:8000` |
| `PCMI_API_KEY` | `testkey123` (dev seed) |

## Install

```bash
make build-mcp          # → bin/pcmi-mcp
make install-mcp        # → $GOPATH/bin/pcmi-mcp
```

## Tools (5)

| MCP tool | Purpose |
|----------|---------|
| `pcmi_store` | Append a memory at `path` with `content` (optional `metadata`, `tags`) |
| `pcmi_retrieve` | Hybrid search under `path_prefix` with optional `query` and `limit` |
| `pcmi_get_history` | All versions for a single `path` |
| `pcmi_list_paths` | Distinct paths under `path_prefix` (via export API) |
| `pcmi_create_link` | Graph edge `from_path` → `to_path` (`link_type`, `metadata`) |

## Resources (2)

| URI | Content |
|-----|---------|
| `pcmi://memory/{path}` | JSON for the current memory at `path` (URL-encode dots if needed) |
| `pcmi://stats` | Tenant stats (`active_memories`, `distilled_count`, …) |

## Protocol

- Transport: **stdio** (one JSON object per line)
- Methods: `initialize`, `tools/list`, `tools/call`, `resources/list`, `resources/read`, `ping`
- Protocol version: `2024-11-05`

Smoke check:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}' \
  | PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123 ./bin/pcmi-mcp 2>/dev/null | grep '"result"'
```

## Client configuration

### Cursor

Add to MCP settings:

```json
{
  "mcpServers": {
    "pcmi": {
      "command": "/absolute/path/to/bin/pcmi-mcp",
      "env": {
        "PCMI_BASE_URL": "http://localhost:8000",
        "PCMI_API_KEY": "your-key"
      }
    }
  }
}
```

### Claude Desktop

Same shape under `mcpServers` in `claude_desktop_config.json`.

## Testing

```bash
make infra-up           # API on :8000 (if not already running)
make test-mcp-unit      # go test ./cmd/mcp/...
make test-mcp-smoke     # initialize handshake (needs bin/pcmi-mcp)
make mcp-e2e            # docker stack + smoke + unit
```

Included in **`make test-full-real`** (Phase 3). See [local-ci.md](local-ci.md).

## Related docs

- [USAGE.md](USAGE.md) — HTTP API reference
- [sdk/HTTP-API.md](../sdk/HTTP-API.md) — endpoint matrix
- [cmd/mcp/README.md](../cmd/mcp/README.md) — binary quick reference
