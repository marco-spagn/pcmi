package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
)

func TestParseEventTypes(t *testing.T) {
	if parseEventTypes("") != nil {
		t.Fatal("empty filter should be nil (allow all)")
	}
	if parseEventTypes("   ") != nil {
		t.Fatal("whitespace-only filter should be nil")
	}
	allowed := parseEventTypes("memory.stored, knowledge.distilled")
	if len(allowed) != 2 {
		t.Fatalf("expected 2 types, got %d", len(allowed))
	}
	if _, ok := allowed[event.EventMemoryStored]; !ok {
		t.Fatal("missing memory.stored")
	}
	if _, ok := allowed["knowledge.distilled"]; !ok {
		t.Fatal("missing knowledge.distilled")
	}
	// trims and skips empty segments
	allowed2 := parseEventTypes(" memory.stored , , ")
	if len(allowed2) != 1 {
		t.Fatalf("expected 1 type after trim, got %d", len(allowed2))
	}
}

func TestEventAllowedTenantFilter(t *testing.T) {
	evt := event.Event{
		Type:    event.EventMemoryStored,
		Payload: map[string]any{"tenant_id": "aaa"},
	}
	if !eventAllowed(evt, nil, "aaa") {
		t.Fatal("expected same tenant to pass")
	}
	if eventAllowed(evt, nil, "bbb") {
		t.Fatal("expected different tenant to be filtered")
	}

	emptyTenant := event.Event{
		Type:    event.EventMemoryStored,
		Payload: map[string]any{"tenant_id": ""},
	}
	if !eventAllowed(emptyTenant, nil, "any") {
		t.Fatal("empty tenant_id in payload should not filter")
	}
	noTenantKey := event.Event{
		Type:    event.EventMemoryStored,
		Payload: map[string]any{"foo": "bar"},
	}
	if !eventAllowed(noTenantKey, nil, "zzz") {
		t.Fatal("missing tenant_id should not filter")
	}
	wrongTypeTenant := event.Event{
		Type:    event.EventMemoryStored,
		Payload: map[string]any{"tenant_id": 123},
	}
	if !eventAllowed(wrongTypeTenant, nil, "zzz") {
		t.Fatal("non-string tenant_id should not filter")
	}

	typeFilter := map[string]struct{}{"other.type": {}}
	if eventAllowed(evt, typeFilter, "aaa") {
		t.Fatal("type not in allow-list should be dropped")
	}
	if !eventAllowed(evt, map[string]struct{}{event.EventMemoryStored: {}}, "aaa") {
		t.Fatal("type in allow-list should pass")
	}
	if eventAllowed(evt, map[string]struct{}{}, "aaa") {
		t.Fatal("non-nil empty allow map should reject every type")
	}
}

func TestEventsHandler_ListSchemas(t *testing.T) {
	h := NewEventsHandler(nil)
	app := fiber.New()
	app.Get("/v1/events/schemas", h.ListSchemas)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/events/schemas", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestEventsHandler_Ingest_invalidJSON(t *testing.T) {
	h := NewEventsHandler(nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000000")
		return c.Next()
	})
	app.Post("/v1/events", h.Ingest)

	req := httptest.NewRequest("POST", "/v1/events", strings.NewReader("{not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

type stubEventIngester struct {
	res *model.IngestEventResponse
	err error
}

func (s stubEventIngester) Ingest(_ context.Context, _ *model.IngestEventRequest, _ string) (*model.IngestEventResponse, error) {
	return s.res, s.err
}

func TestEventsHandler_Ingest_missingEventType(t *testing.T) {
	h := NewEventsHandler(nil)
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000001")
		return c.Next()
	})
	app.Post("/v1/events", h.Ingest)

	req := httptest.NewRequest("POST", "/v1/events", strings.NewReader(`{"payload":{}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestEventsHandler_Ingest_serviceErrorWithMissingKeyword(t *testing.T) {
	h := &EventsHandler{ingest: stubEventIngester{err: errors.New("validation missing: schema")}}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000002")
		return c.Next()
	})
	app.Post("/v1/events", h.Ingest)

	req := httptest.NewRequest("POST", "/v1/events", strings.NewReader(`{"event_type":"x","payload":{}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestEventsHandler_Ingest_serviceInternalError(t *testing.T) {
	h := &EventsHandler{ingest: stubEventIngester{err: errors.New("insert failed")}}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000003")
		return c.Next()
	})
	app.Post("/v1/events", h.Ingest)

	req := httptest.NewRequest("POST", "/v1/events", strings.NewReader(`{"event_type":"x","payload":{}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
}

func TestEventsHandler_Ingest_success(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	h := &EventsHandler{ingest: stubEventIngester{res: &model.IngestEventResponse{
		ID:        "e1",
		EventType: "memory.stored",
		Timestamp: ts,
		Status:    "ingested",
	}}}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000004")
		return c.Next()
	})
	app.Post("/v1/events", h.Ingest)

	req := httptest.NewRequest("POST", "/v1/events", strings.NewReader(`{"event_type":"memory.stored","payload":{"k":1}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var got model.IngestEventResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "e1" || got.EventType != "memory.stored" || got.Status != "ingested" {
		t.Fatalf("unexpected %+v", got)
	}
}
