package middleware

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func newTenantApp() *fiber.App {
	app := fiber.New()
	app.Use(TenantMiddleware())
	app.Get("/ok", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"tenant": c.Locals(TenantContextKey)})
	})
	return app
}

func TestTenantMiddlewareMissingHeader(t *testing.T) {
	app := newTenantApp()
	req := httptest.NewRequest("GET", "/ok", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing tenant, got %d", resp.StatusCode)
	}
}

func TestTenantMiddlewareInvalidUUID(t *testing.T) {
	app := newTenantApp()
	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set("X-Tenant-ID", "not-a-uuid")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for invalid UUID, got %d", resp.StatusCode)
	}
}

func TestTenantMiddlewareValidUUID(t *testing.T) {
	app := newTenantApp()
	tid := "550e8400-e29b-41d4-a716-446655440000"
	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set("X-Tenant-ID", tid)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for valid UUID, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(body, &out)
	if out["tenant"] != tid {
		t.Fatalf("expected tenant=%s in locals, got %v", tid, out["tenant"])
	}
}

func TestTenantMiddlewareFromQueryParam(t *testing.T) {
	app := newTenantApp()
	tid := "550e8400-e29b-41d4-a716-446655440001"
	req := httptest.NewRequest("GET", "/ok?tenant_id="+tid, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for valid UUID query param, got %d", resp.StatusCode)
	}
}

func TestTenantMiddlewareHeaderTakesPrecedence(t *testing.T) {
	app := newTenantApp()
	headerTid := "550e8400-e29b-41d4-a716-446655440002"
	req := httptest.NewRequest("GET", "/ok?tenant_id=bad-uuid", nil)
	req.Header.Set("X-Tenant-ID", headerTid)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	// Header is non-empty → used first → valid UUID → 200
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 when header overrides bad query param, got %d", resp.StatusCode)
	}
}
