package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

func TestWebhookRegisterMissingTenantContext(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/hook", NewWebhookHandler(nil).Register)

	req := httptest.NewRequest("POST", "/hook", strings.NewReader(`{"url":"https://example.com/h"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestWebhookListMissingTenantContext(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/hooks", NewWebhookHandler(nil).List)

	resp, err := app.Test(httptest.NewRequest("GET", "/hooks", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestWebhookDeadLetterMissingTenantContext(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/dlq", NewWebhookHandler(nil).DeadLetter)

	resp, err := app.Test(httptest.NewRequest("GET", "/dlq", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestWebhookRegister_invalidJSON(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000000")
		return c.Next()
	})
	app.Post("/hook", NewWebhookHandler(nil).Register)

	req := httptest.NewRequest("POST", "/hook", strings.NewReader(`{"url":`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestWebhookRegister_missingURL(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "00000000-0000-0000-0000-000000000000")
		return c.Next()
	})
	app.Post("/hook", NewWebhookHandler(nil).Register)

	req := httptest.NewRequest("POST", "/hook", strings.NewReader(`{"event_types":["memory.stored"]}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}
