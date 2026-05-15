package middleware

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRateLimitMiddlewareBlocksBurst(t *testing.T) {
	t.Setenv("RATE_LIMIT_DISABLED", "")
	t.Setenv("RATE_LIMIT_RPM", "60")
	t.Setenv("RATE_LIMIT_BURST", "2")
	rateLimitRPM = 0 // reset init

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(APIKeyIDContextKey, "test-key-id")
		return c.Next()
	})
	app.Use(RateLimitMiddleware())
	app.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/ping", nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("request %d: want 200 got %d", i+1, resp.StatusCode)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	req := httptest.NewRequest("GET", "/ping", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 429 {
		t.Fatalf("want 429 got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRateLimitDisabled(t *testing.T) {
	t.Setenv("RATE_LIMIT_DISABLED", "true")
	rateLimitRPM = 0

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(APIKeyIDContextKey, "k")
		return c.Next()
	})
	app.Use(RateLimitMiddleware())
	app.Get("/ping", func(c *fiber.Ctx) error { return c.SendString("ok") })

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/ping", nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("want 200 got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
}
