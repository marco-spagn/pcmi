package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

// ─── GET /v1/events/schemas ──────────────────────────────────────────────────

func TestEventsListSchemasReturnsSchemas(t *testing.T) {
	app := newTestApp("tid", "admin")
	eh := NewEventsHandler(nil)
	app.Get("/v1/events/schemas", eh.ListSchemas)

	req := httptest.NewRequest("GET", "/v1/events/schemas", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	total, _ := out["total"].(float64)
	if total < 1 {
		t.Fatalf("expected at least 1 schema, got %v", total)
	}
}

// ─── eventAllowed ─────────────────────────────────────────────────────────────

func TestEventAllowedNoFilter(t *testing.T) {
	evt := event.Event{Type: event.EventMemoryStored, Payload: map[string]any{"tenant_id": "tid"}}
	if !eventAllowed(evt, nil, "tid") {
		t.Fatal("nil allowed map should pass all events for matching tenant")
	}
}

func TestEventAllowedFilterMatch(t *testing.T) {
	allowed := parseEventTypes("memory.stored")
	evt := event.Event{Type: event.EventMemoryStored, Payload: map[string]any{"tenant_id": "tid"}}
	if !eventAllowed(evt, allowed, "tid") {
		t.Fatal("matching type and tenant should be allowed")
	}
}

func TestEventAllowedFilterNoMatch(t *testing.T) {
	allowed := parseEventTypes("knowledge.distilled")
	evt := event.Event{Type: event.EventMemoryStored, Payload: map[string]any{"tenant_id": "tid"}}
	if eventAllowed(evt, allowed, "tid") {
		t.Fatal("non-matching type should be filtered")
	}
}

func TestEventAllowedEmptyTenantAllowsAll(t *testing.T) {
	evt := event.Event{Type: event.EventMemoryStored, Payload: map[string]any{"tenant_id": "other"}}
	if !eventAllowed(evt, nil, "") {
		t.Fatal("empty tenantID should pass all events")
	}
}

func TestEventAllowedPayloadTenantMismatch(t *testing.T) {
	evt := event.Event{Type: event.EventMemoryStored, Payload: map[string]any{"tenant_id": "other"}}
	if eventAllowed(evt, nil, "tid") {
		t.Fatal("payload tenant_id mismatch should be filtered")
	}
}

// ─── WebhookHandler.Register — URL missing ────────────────────────────────────

func TestWebhookRegisterMissingURL(t *testing.T) {
	app := newTestApp("tid", "admin")
	wh := NewWebhookHandler(nil)
	app.Post("/v1/webhooks", wh.Register)

	req := httptest.NewRequest("POST", "/v1/webhooks", strings.NewReader(`{"event_types":["memory.stored"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing URL, got %d", resp.StatusCode)
	}
}

// ─── POST /v1/memories/rollback ───────────────────────────────────────────────

func TestRollbackHandlerMissingVersionAndAsOf(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tid", "admin")
	repo := &handlerMockRepo{}
	svc := service.NewMemoryService(repo, nil)
	api := app.Group("/v1")
	api.Post("/memories/rollback", func(c *fiber.Ctx) error {
		var req model.RollbackRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Version == nil && req.AsOf == nil {
			return c.Status(400).JSON(fiber.Map{"error": "version or as_of is required"})
		}
		if req.Version != nil && req.AsOf != nil {
			return c.Status(400).JSON(fiber.Map{"error": "provide only one of version or as_of"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Rollback(c.Context(), &req, tenantID)
		if err != nil {
			if strings.Contains(err.Error(), "no historical version") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/v1/memories/rollback", strings.NewReader(`{"path":"root.x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing version/as_of, got %d", resp.StatusCode)
	}
}

func TestRollbackHandlerBothVersionAndAsOf(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tid", "admin")
	repo := &handlerMockRepo{}
	svc := service.NewMemoryService(repo, nil)
	api := app.Group("/v1")
	api.Post("/memories/rollback", func(c *fiber.Ctx) error {
		var req model.RollbackRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Version == nil && req.AsOf == nil {
			return c.Status(400).JSON(fiber.Map{"error": "version or as_of is required"})
		}
		if req.Version != nil && req.AsOf != nil {
			return c.Status(400).JSON(fiber.Map{"error": "provide only one of version or as_of"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Rollback(c.Context(), &req, tenantID)
		if err != nil {
			if strings.Contains(err.Error(), "no historical version") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	body := `{"path":"root.x","version":1,"as_of":"2024-01-01T00:00:00Z"}`
	req := httptest.NewRequest("POST", "/v1/memories/rollback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for both version+as_of, got %d", resp.StatusCode)
	}
}

func TestRollbackHandlerNotFound(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tid", "admin")
	repo := &handlerMockRepo{}
	svc := service.NewMemoryService(repo, nil)
	api := app.Group("/v1")
	api.Post("/memories/rollback", func(c *fiber.Ctx) error {
		var req model.RollbackRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Version == nil && req.AsOf == nil {
			return c.Status(400).JSON(fiber.Map{"error": "version or as_of is required"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Rollback(c.Context(), &req, tenantID)
		if err != nil {
			if strings.Contains(err.Error(), "no historical version") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	v := 1
	_ = v
	body := `{"path":"root.x","version":2}`
	req := httptest.NewRequest("POST", "/v1/memories/rollback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	// mock returns "no historical version" → 404
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
