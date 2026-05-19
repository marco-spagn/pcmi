package middleware

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marco-spagn/pcmi/internal/config"
)

func newRateLimitApp(role string, cfg *config.Config) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(APIKeyIDContextKey, "key-test")
		if role != "" {
			c.Locals(RoleContextKey, role)
		}
		return c.Next()
	})
	app.Use(RateLimitMiddleware(cfg))
	app.Get("/ping", func(c *fiber.Ctx) error { return c.SendString("ok") })
	return app
}

func hitN(t *testing.T, app *fiber.App, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		req := httptest.NewRequest("GET", "/ping", nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, resp.StatusCode)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func TestRoleLimitForReadonly(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM_READONLY", "150")
	cfg := config.Load()
	if got := RoleLimitFor("readonly", cfg); got != 150 {
		t.Fatalf("expected 150, got %d", got)
	}
}

func TestRoleLimitForWrite(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM_WRITE", "80")
	cfg := config.Load()
	if got := RoleLimitFor("write", cfg); got != 80 {
		t.Fatalf("expected 80, got %d", got)
	}
}

func TestRoleLimitForUser(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM_WRITE", "75")
	cfg := config.Load()
	if got := RoleLimitFor("user", cfg); got != 75 {
		t.Fatalf("expected 75 for user role (same bucket as write), got %d", got)
	}
}

func TestRoleLimitForAdmin(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM_ADMIN", "10")
	cfg := config.Load()
	if got := RoleLimitFor("admin", cfg); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestRoleLimitForUnknown(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM", "55")
	cfg := config.Load()
	if got := RoleLimitFor("unknown", cfg); got != 55 {
		t.Fatalf("expected fallback 55, got %d", got)
	}
}

func TestRoleLimitForDefault(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM_READONLY", "")
	t.Setenv("RATE_LIMIT_RPM_WRITE", "")
	t.Setenv("RATE_LIMIT_RPM_ADMIN", "")
	t.Setenv("RATE_LIMIT_RPM", "")
	cfg := config.Load()
	if got := RoleLimitFor("readonly", cfg); got != 200 {
		t.Fatalf("readonly default: expected 200, got %d", got)
	}
	if got := RoleLimitFor("admin", cfg); got != 30 {
		t.Fatalf("admin default: expected 30, got %d", got)
	}
	if got := RoleLimitFor("write", cfg); got != 100 {
		t.Fatalf("write default: expected 100, got %d", got)
	}
	if got := RoleLimitFor("", cfg); got != 120 {
		t.Fatalf("fallback default: expected 120, got %d", got)
	}
}

func TestRateLimitAdminBlocksFaster(t *testing.T) {
	t.Setenv("RATE_LIMIT_DISABLED", "")
	t.Setenv("RATE_LIMIT_RPM_ADMIN", "2")
	cfg := config.Load()

	app := newRateLimitApp("admin", cfg)

	hitN(t, app, 2)

	req := httptest.NewRequest("GET", "/ping", nil)
	resp, _ := app.Test(req, -1)
	if resp.StatusCode != 429 {
		t.Fatalf("admin: expected 429 after 2 requests, got %d", resp.StatusCode)
	}
}

func TestRateLimitReadonlyHigherLimit(t *testing.T) {
	t.Setenv("RATE_LIMIT_DISABLED", "")
	t.Setenv("RATE_LIMIT_RPM_READONLY", "5")
	cfg := config.Load()

	app := newRateLimitApp("readonly", cfg)
	hitN(t, app, 5)

	req := httptest.NewRequest("GET", "/ping", nil)
	resp, _ := app.Test(req, -1)
	if resp.StatusCode != 429 {
		t.Fatalf("readonly: expected 429 after 5 requests, got %d", resp.StatusCode)
	}
}

func TestRateLimitRolesAreIndependent(t *testing.T) {
	t.Setenv("RATE_LIMIT_DISABLED", "")
	t.Setenv("RATE_LIMIT_RPM_ADMIN", "1")
	t.Setenv("RATE_LIMIT_RPM_READONLY", "5")
	cfg := config.Load()

	adminApp := newRateLimitApp("admin", cfg)
	readonlyApp := newRateLimitApp("readonly", cfg)

	hitN(t, adminApp, 1)
	req := httptest.NewRequest("GET", "/ping", nil)
	resp, _ := adminApp.Test(req, -1)
	if resp.StatusCode != 429 {
		t.Fatal("admin bucket should be exhausted after 1 request")
	}

	req2 := httptest.NewRequest("GET", "/ping", nil)
	resp2, _ := readonlyApp.Test(req2, -1)
	if resp2.StatusCode != 200 {
		t.Fatalf("readonly should not be affected by admin exhaustion, got %d", resp2.StatusCode)
	}
}
