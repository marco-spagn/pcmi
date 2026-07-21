package handler_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/handler"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

func TestExtractionHandler_disabledRun(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000000")
		c.Locals(middleware.RoleContextKey, "write")
		return c.Next()
	})
	cfg := &config.Config{ExtractionEnabled: false}
	handler.RegisterExtractionRoutes(app, nil, nil, cfg, nil)

	req := httptest.NewRequest("POST", "/v1/memories/extraction/42", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
}

func TestExtractionHandler_invalidMemoryID(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000000")
		c.Locals(middleware.RoleContextKey, "write")
		return c.Next()
	})
	cfg := &config.Config{ExtractionEnabled: true}
	handler.RegisterExtractionRoutes(app, nil, nil, cfg, nil)

	req := httptest.NewRequest("POST", "/v1/memories/extraction/not-a-number", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestExtractionHandler_upsertProfileBadJSON(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000000")
		c.Locals(middleware.RoleContextKey, "write")
		return c.Next()
	})
	cfg := &config.Config{}
	handler.RegisterExtractionRoutes(app, nil, nil, cfg, nil)

	req := httptest.NewRequest("PUT", "/v1/extraction-profiles/soc.siem.v1", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
