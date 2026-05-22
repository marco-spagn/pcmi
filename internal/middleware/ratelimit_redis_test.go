package middleware

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/event"
)

func setupRedisRateLimit(t *testing.T, env func()) *fiber.App {
	t.Helper()
	event.TestLockRedis(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	event.InitRedis(mr.Addr())

	t.Setenv("RATE_LIMIT_DISABLED", "")
	t.Setenv("RATE_LIMIT_BACKEND", "redis")
	t.Setenv("RATE_LIMIT_WINDOW_SECS", "60")
	if env != nil {
		env()
	}
	cfg := config.Load()

	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(APIKeyIDContextKey, "key-test")
		if role := c.Get("X-Test-Role"); role != "" {
			c.Locals(RoleContextKey, role)
		}
		return c.Next()
	})
	app.Use(RateLimitMiddleware(cfg))
	app.Get("/ping", func(c *fiber.Ctx) error { return c.SendString("ok") })
	for _, probe := range []string{"/health", "/v1/health", "/metrics", "/ready", "/v1/ready"} {
		p := probe
		app.Get(p, func(c *fiber.Ctx) error { return c.SendString("probe") })
	}
	return app
}

func TestRateLimitMiddleware_ProbesExempt(t *testing.T) {
	app := setupRedisRateLimit(t, func() {
		t.Setenv("RATE_LIMIT_RPM_ADMIN", "1")
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/health", nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("probe request %d: want 200 got %d", i+1, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestRateLimitMiddleware_AdminKeyHigherLimit(t *testing.T) {
	t.Setenv("RATE_LIMIT_DISABLED", "")
	t.Setenv("RATE_LIMIT_BACKEND", "redis")
	t.Setenv("RATE_LIMIT_WINDOW_SECS", "60")
	t.Setenv("RATE_LIMIT_RPM_ADMIN", "1")
	t.Setenv("RATE_LIMIT_RPM_READONLY", "5")

	event.TestLockRedis(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	event.InitRedis(mr.Addr())
	cfg := config.Load()

	adminApp := newRateLimitApp("admin", cfg)
	readonlyApp := newRateLimitApp("readonly", cfg)

	hitN(t, adminApp, 1)
	req := httptest.NewRequest("GET", "/ping", nil)
	resp, _ := adminApp.Test(req, -1)
	if resp.StatusCode != 429 {
		t.Fatalf("admin: expected 429 after 1 request, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	req2 := httptest.NewRequest("GET", "/ping", nil)
	resp2, err := readonlyApp.Test(req2, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("readonly should allow more than admin, got %d", resp2.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
}
