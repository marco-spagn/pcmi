package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestAuditMiddlewareSkipsReadyProbe(t *testing.T) {
	// Nil DB: must not run audit insert (skip path before Exec).
	mw := NewAuditMiddleware(nil)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(mw.Middleware())
	app.Get("/ready", func(c *fiber.Ctx) error {
		return c.Status(200).SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/ready", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAuditMiddlewareSkipsV1Ready(t *testing.T) {
	mw := NewAuditMiddleware(nil)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(mw.Middleware())
	app.Get("/v1/ready", func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/ready", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestAuditMiddlewareEmptyTenantSkipsDB(t *testing.T) {
	mw := NewAuditMiddleware(nil)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(mw.Middleware())
	app.Get("/private", func(c *fiber.Ctx) error {
		return c.SendStatus(204)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/private", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 204 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
