package handler

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

func TestDistillationPolicy_Create_ValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing tenant context", func(t *testing.T) {
		t.Parallel()
		h := &DistillationPolicyHandler{}
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Post("/v1/distillation/policies", h.Create)

		req := httptest.NewRequest("POST", "/v1/distillation/policies",
			strings.NewReader(`{"name":"test","path_prefix":"a.b"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid json body", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Post("/v1/distillation/policies", h.Create)

		req := httptest.NewRequest("POST", "/v1/distillation/policies", strings.NewReader(`{broken`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Post("/v1/distillation/policies", h.Create)

		req := httptest.NewRequest("POST", "/v1/distillation/policies",
			strings.NewReader(`{"path_prefix":"a.b","name":""}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("missing path_prefix", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Post("/v1/distillation/policies", h.Create)

		req := httptest.NewRequest("POST", "/v1/distillation/policies",
			strings.NewReader(`{"path_prefix":"","name":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestDistillationPolicy_Patch_ValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing tenant context", func(t *testing.T) {
		t.Parallel()
		h := &DistillationPolicyHandler{}
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Patch("/v1/distillation/policies/:id", h.Patch)

		req := httptest.NewRequest("PATCH", "/v1/distillation/policies/1",
			strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid id param", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Patch("/v1/distillation/policies/:id", h.Patch)

		req := httptest.NewRequest("PATCH", "/v1/distillation/policies/abc", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("zero id", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Patch("/v1/distillation/policies/:id", h.Patch)

		req := httptest.NewRequest("PATCH", "/v1/distillation/policies/0", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d (id=0)", resp.StatusCode)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Patch("/v1/distillation/policies/:id", h.Patch)

		req := httptest.NewRequest("PATCH", "/v1/distillation/policies/1", strings.NewReader(`{`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestDistillationPolicy_List_ValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing tenant context", func(t *testing.T) {
		t.Parallel()
		h := &DistillationPolicyHandler{}
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Get("/v1/distillation/policies", h.List)

		req := httptest.NewRequest("GET", "/v1/distillation/policies", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid pagination", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Get("/v1/distillation/policies", h.List)

		req := httptest.NewRequest("GET", "/v1/distillation/policies?cursor=invalid&after_id=1", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d body=%s", resp.StatusCode, body)
		}
	})
}

func TestDistillationPolicy_ListRuns_ValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing tenant context", func(t *testing.T) {
		t.Parallel()
		h := &DistillationPolicyHandler{}
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Get("/v1/distillation/runs", h.ListRuns)

		req := httptest.NewRequest("GET", "/v1/distillation/runs", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("want 401, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid policy_id query param", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
			return c.Next()
		})
		h := &DistillationPolicyHandler{}
		app.Get("/v1/distillation/runs", h.ListRuns)

		req := httptest.NewRequest("GET", "/v1/distillation/runs?policy_id=abc", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestDistillationPolicy_NewHandler(t *testing.T) {
	t.Parallel()
	h := NewDistillationPolicyHandler(nil)
	if h == nil {
		t.Fatal("NewDistillationPolicyHandler returned nil")
	}
	if h.db != nil {
		t.Fatal("expected nil db")
	}
}

func TestWebhookHandler_NewHandler(t *testing.T) {
	t.Parallel()
	h := NewWebhookHandler(nil)
	if h == nil {
		t.Fatal("NewWebhookHandler returned nil")
	}
}

func TestWebhookHandler_List_InvalidPagination(t *testing.T) {
	t.Parallel()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		return c.Next()
	})
	app.Get("/hooks", NewWebhookHandler(nil).List)

	req := httptest.NewRequest("GET", "/hooks?cursor=bad&after_id=1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_DeadLetter_InvalidPagination(t *testing.T) {
	t.Parallel()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		return c.Next()
	})
	app.Get("/dlq", NewWebhookHandler(nil).DeadLetter)

	req := httptest.NewRequest("GET", "/dlq?cursor=bad&after_id=1", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_DeadLetter_AfterIDRejected(t *testing.T) {
	t.Parallel()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		return c.Next()
	})
	app.Get("/dlq", NewWebhookHandler(nil).DeadLetter)

	req := httptest.NewRequest("GET", "/dlq?after_id=99", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_List_AfterIDRejected(t *testing.T) {
	t.Parallel()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
		return c.Next()
	})
	app.Get("/hooks", NewWebhookHandler(nil).List)

	req := httptest.NewRequest("GET", "/hooks?after_id=99", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}
