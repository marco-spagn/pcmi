package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/service"
)

// ─── /v1/health ──────────────────────────────────────────────────────────────

func TestHealthEndpointNoDBRequired(t *testing.T) {
	// health route reads dbWrite.Stat() which panics on nil pool;
	// test the version.Tag path only via a stub handler.
	// Full health is covered by integration smoke.
	t.Skip("health requires live pool — covered in integration smoke")
}

// ─── /v1/memories/refine ─────────────────────────────────────────────────────
// (covered in refine_handler_test.go)

// ─── /v1/audit ───────────────────────────────────────────────────────────────

func TestAuditHandlerInvalidSince(t *testing.T) {
	app := newTestApp("tid", "admin")

	// Attach a nil-repo handler; the bad-timestamp check fires before any DB call.
	app.Get("/v1/audit", NewAuditHandler(nil).List)

	req := httptest.NewRequest("GET", "/v1/audit?since=not-a-timestamp", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for bad since, got %d", resp.StatusCode)
	}
}

// ─── /v1/memories/history ────────────────────────────────────────────────────

func TestHistoryHandlerMissingPath(t *testing.T) {
	app := newTestApp("tid", "admin")
	app.Get("/v1/memories/history", NewHistoryHandler(nil, nil).Get)

	req := httptest.NewRequest("GET", "/v1/memories/history", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing path, got %d", resp.StatusCode)
	}
}

// ─── /v1/lineage ─────────────────────────────────────────────────────────────

func TestLineageHandlerMissingPath(t *testing.T) {
	app := newTestApp("tid", "admin")
	lh := NewLineageHandler(nil, nil)
	app.Get("/v1/lineage/memory", lh.MemoryLineage)

	req := httptest.NewRequest("GET", "/v1/lineage/memory", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing path, got %d", resp.StatusCode)
	}
}

func TestDistilledLineageHandlerBadID(t *testing.T) {
	app := newTestApp("tid", "admin")
	lh := NewLineageHandler(nil, nil)
	app.Get("/v1/lineage/distilled/:id", lh.DistilledLineage)

	req := httptest.NewRequest("GET", "/v1/lineage/distilled/not-a-number", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for bad id, got %d", resp.StatusCode)
	}
}

func TestDistilledLineageHandlerEmptyID(t *testing.T) {
	app := newTestApp("tid", "admin")
	lh := NewLineageHandler(nil, nil)
	// Route with id="" — Fiber won't match :id with empty string so test via custom route.
	app.Get("/v1/lineage/distilled/", lh.DistilledLineage)

	req := httptest.NewRequest("GET", "/v1/lineage/distilled/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	// Either 400 (empty id) or 404 (route not matched); not 200 or 500.
	if resp.StatusCode == 200 || resp.StatusCode == 500 {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
}

// ─── /v1/memories/links ──────────────────────────────────────────────────────

func TestLinksPostHandlerBadJSON(t *testing.T) {
	app := newTestApp("tid", "admin")
	lnh := &LinksHandler{repo: nil}
	app.Post("/v1/memories/links", lnh.Post)

	req := httptest.NewRequest("POST", "/v1/memories/links", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

// ─── /v1/distilled ───────────────────────────────────────────────────────────

func TestDistilledHandlerMissingPathPrefix(t *testing.T) {
	app := newTestApp("tid", "admin")
	app.Get("/v1/distilled", NewDistilledHandler(nil).Get)

	req := httptest.NewRequest("GET", "/v1/distilled", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing path_prefix, got %d", resp.StatusCode)
	}
}

// ─── /v1/events ──────────────────────────────────────────────────────────────

func TestEventsIngestHandlerBadJSON(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tid", "admin")
	eh := &EventsHandler{ingest: service.NewEventService(nil)}
	app.Post("/v1/events", eh.Ingest)

	req := httptest.NewRequest("POST", "/v1/events", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

// ─── /v1/memories/summarize ──────────────────────────────────────────────────

func TestSummarizeHandlerBadJSON(t *testing.T) {
	app := newTestApp("tid", "admin")
	sh := NewSummarizeHandler(nil, nil)
	app.Post("/v1/memories/summarize", sh.Post)

	req := httptest.NewRequest("POST", "/v1/memories/summarize", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

// ─── /v1/webhooks ────────────────────────────────────────────────────────────

func TestWebhookRegisterBadJSON(t *testing.T) {
	app := newTestApp("tid", "admin")
	wh := NewWebhookHandler(nil)
	app.Post("/v1/webhooks", wh.Register)

	req := httptest.NewRequest("POST", "/v1/webhooks", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

func readBodyJSON(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("could not decode body: %v", err)
	}
	return m
}
