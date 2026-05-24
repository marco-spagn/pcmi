# PCMI Go SDK

Thin **HTTP** client for PCMI, aligned with the Python and TypeScript SDKs. It does not speak gRPC; for high-throughput workloads use gRPC (`proto/pcmi/v1/memory.proto`) or REST directly.

## Requirements

- Go 1.25+

## Install

From the repo root (replace with your module path when published):

```bash
cd sdk/go
go test ./...
```

In another module:

```go
import "github.com/marco-spagn/pcmi/sdk/go/pcmi"
```

## Quick start

```bash
export PCMI_BASE_URL=http://localhost:8000 PCMI_API_KEY=testkey123
go run ./examples/basic
```

```go
package main

import (
    "context"
    "log"

    "github.com/marco-spagn/pcmi/sdk/go/pcmi"
)

func main() {
    c, err := pcmi.NewClient("http://localhost:8000", "your-key")
    if err != nil {
        log.Fatal(err)
    }
    ctx := context.Background()
    _, err = c.Store(ctx, "root.demo", "hello", nil, &pcmi.StoreOptions{Tags: []string{"sdk"}})
    if err != nil {
        log.Fatal(err)
    }
    out, err := c.Retrieve(ctx, "root.demo", "", 5, nil)
    if err != nil {
        log.Fatal(err)
    }
    log.Println("total:", out.Total)
}
```

## API surface

| Package file | Coverage |
|--------------|----------|
| `memory.go` | store, retrieve, batch, compact, refine, history, stats |
| `session.go` | create/end session, working memory, promote |
| `admin.go` | tenants, API keys (`ListTenants` / `ListAPIKeys` accept `limit` + `cursor`) |
| `events.go` | ingest, schemas, SSE subscribe |
| `options.go` | client timeout, retries, request options |
| `errors.go` | typed API errors |

## SSE events

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

ch, err := client.Subscribe(ctx, &pcmi.SubscribeOptions{Types: []string{"memory.stored"}})
if err != nil {
    log.Fatal(err)
}
for ev := range ch {
    log.Println(ev.Type, ev.Payload)
}
```

## Makefile targets (repo root)

```bash
make sdk-go-test    # unit tests with -race
make sdk-go-smoke   # infra-up + tests + examples/basic + infra-down
make sdk-all        # Python/TS smoke + Go tests
```

## List pagination

`ListTenants(ctx, limit, cursor)` and `ListAPIKeys(ctx, tenantID, limit, cursor)` map to `GET /v1/admin/*` query params. The JSON body includes `next_cursor`, `has_more`, and `total` on tenants only. Other list endpoints are not wrapped yet — call REST with the same query params (see [USAGE.md](../../docs/USAGE.md#paginazione-cursor-sulle-liste-pcmi-014)).

## OpenAPI

Full schemas: [`../../docs/openapi.yaml`](../../docs/openapi.yaml).

## Versioning

SDK releases are independent of the server API tag (`v1.x.y` on `/v1/health`). Align request bodies with OpenAPI when upgrading servers.
