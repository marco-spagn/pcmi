package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestAPIKeyMiddleware_skipsPublicGETRoutesWithoutDB(t *testing.T) {
	paths := []string{"/health", "/v1/health", "/metrics", "/ready", "/v1/ready"}
	for _, p := range paths {
		p := p
		t.Run(p, func(t *testing.T) {
			app := fiber.New()
			app.Use(APIKeyMiddleware(nil))
			app.Get(p, func(c *fiber.Ctx) error { return c.SendString("ok") })

			req := httptest.NewRequest(http.MethodGet, p, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("status %d for %s", resp.StatusCode, p)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != "ok" {
				t.Fatalf("body %q", body)
			}
		})
	}
}

func TestAPIKeyMiddleware_missingKeyOnProtectedRoute(t *testing.T) {
	app := fiber.New()
	app.Use(APIKeyMiddleware(nil))
	app.Get("/v1/memories/foo", func(c *fiber.Ctx) error {
		return c.SendString("should-not-run")
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/memories/foo", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status %d want 401", resp.StatusCode)
	}
}
