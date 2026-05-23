# pcmi-mcp

stdio [Model Context Protocol](https://modelcontextprotocol.io/) server for PCMI — exposes memory store/retrieve, history, path listing, links, and read-only resources for AI agents (Cursor, Claude Desktop, etc.).

## Environment

| Variable | Description |
|----------|-------------|
| `PCMI_BASE_URL` | PCMI HTTP API base (e.g. `http://localhost:8000`) |
| `PCMI_API_KEY` | Tenant API key (`X-API-Key`) |

## Build & test

```bash
make build-mcp
make test-mcp-unit
make test-mcp-smoke   # initialize handshake only
make mcp-e2e          # infra-up + smoke + unit tests
```

## Tools

| Tool | HTTP mapping |
|------|----------------|
| `pcmi_store` | `POST /v1/memories` |
| `pcmi_retrieve` | `POST /v1/retrieve` |
| `pcmi_get_history` | `GET /v1/memories/history` |
| `pcmi_list_paths` | `POST /v1/memories/export` (distinct paths) |
| `pcmi_create_link` | `POST /v1/memories/links` |

## Resources

| URI | HTTP mapping |
|-----|----------------|
| `pcmi://memory/{path}` | `GET /v1/memories/{path}` |
| `pcmi://stats` | `GET /v1/stats` |

## Cursor / Claude config (example)

```json
{
  "mcpServers": {
    "pcmi": {
      "command": "/path/to/bin/pcmi-mcp",
      "env": {
        "PCMI_BASE_URL": "http://localhost:8000",
        "PCMI_API_KEY": "testkey123"
      }
    }
  }
}
```

See [docs/MCP.md](../../docs/MCP.md) for the full integration guide.
