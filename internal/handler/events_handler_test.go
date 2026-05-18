package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
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
