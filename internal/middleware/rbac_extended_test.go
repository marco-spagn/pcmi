package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRequireWriteRoleAdminAllowed(t *testing.T) {
	app := fiber.New()
	app.Post("/write", func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "admin")
		return RequireWriteRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(201)
	})

	req := httptest.NewRequest("POST", "/write", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201 for admin role, got %d", resp.StatusCode)
	}
}

func TestRequireWriteRoleMissingRoleAllowed(t *testing.T) {
	// No role set → empty string → should NOT be blocked (only "readonly" is blocked)
	app := fiber.New()
	app.Post("/write", func(c *fiber.Ctx) error {
		// Do NOT set RoleContextKey
		return RequireWriteRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("POST", "/write", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for missing role, got %d", resp.StatusCode)
	}
}

func TestRequireWriteRoleReadonlyIs403(t *testing.T) {
	app := fiber.New()
	app.Delete("/del", func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "readonly")
		return RequireWriteRole(c)
	}, func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("DELETE", "/del", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
