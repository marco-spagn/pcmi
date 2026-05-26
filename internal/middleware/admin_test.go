package middleware

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRequireAdminRole(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "user")
		return c.Next()
	})
	app.Get("/admin", RequireAdminRole, func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/admin", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("expected body")
	}
}

func TestRequireAdminRoleIfAPIKeyPresent(t *testing.T) {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if c.Get("X-API-Key") != "" {
			c.Locals(RoleContextKey, "user")
		}
		return c.Next()
	})
	app.Get("/ui", RequireAdminRoleIfAPIKeyPresent, func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/ui", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("no key: status=%d want 200", resp.StatusCode)
	}

	req = httptest.NewRequest("GET", "/ui", nil)
	req.Header.Set("X-API-Key", "secret")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("user key: status=%d want 403", resp.StatusCode)
	}
}
