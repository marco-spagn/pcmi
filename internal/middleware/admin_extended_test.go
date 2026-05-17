package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRequireAdminRoleGranted(t *testing.T) {
	app := fiber.New()
	app.Get("/admin", func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "admin")
		return RequireAdminRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/admin", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for admin role, got %d", resp.StatusCode)
	}
}

func TestRequireAdminRoleReadonly(t *testing.T) {
	app := fiber.New()
	app.Get("/admin", func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "readonly")
		return RequireAdminRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/admin", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for readonly role, got %d", resp.StatusCode)
	}
}

func TestRequireAdminRoleNoRole(t *testing.T) {
	app := fiber.New()
	app.Get("/admin", func(c *fiber.Ctx) error {
		// No role set
		return RequireAdminRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("GET", "/admin", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for empty role, got %d", resp.StatusCode)
	}
}
