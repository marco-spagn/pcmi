package middleware

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func TestIdempotencyMiddleware_NoKey(t *testing.T) {
	t.Parallel()

	cache := &fakeIdempotencyCache{}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(NewIdempotencyMiddleware(cache))
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		return c.Status(201).JSON(fiber.Map{"id": 1})
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
}

func TestIdempotencyMiddleware_InvalidKey(t *testing.T) {
	t.Parallel()

	cache := &fakeIdempotencyCache{}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(NewIdempotencyMiddleware(cache))
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	req.Header.Set("X-Idempotency-Key", "not-a-uuid")
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

func TestIdempotencyMiddleware_CacheHit(t *testing.T) {
	t.Parallel()

	cached := json.RawMessage(`{"id":42,"status":"stored"}`)
	cache := &fakeIdempotencyCache{getResult: cached, getOK: true}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(TenantContextKey, uuid.New().String())
		return c.Next()
	})
	app.Use(NewIdempotencyMiddleware(cache))
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		return c.Status(201).JSON(fiber.Map{"id": 99})
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	req.Header.Set("X-Idempotency-Key", uuid.New().String())
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result["id"] != float64(42) {
		t.Fatalf("want cached response id=42, got %v", result)
	}
	if resp.Header.Get("X-Idempotency-Replayed") != "true" {
		t.Fatal("missing X-Idempotency-Replayed header")
	}
}

func TestIdempotencyMiddleware_CacheMiss(t *testing.T) {
	t.Parallel()

	cache := &fakeIdempotencyCache{}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(TenantContextKey, uuid.New().String())
		return c.Next()
	})
	app.Use(NewIdempotencyMiddleware(cache))
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		return c.Status(200).JSON(fiber.Map{"id": 5, "status": "stored"})
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	req.Header.Set("X-Idempotency-Key", uuid.New().String())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	if cache.putCalled == 0 {
		t.Fatal("expected cache.Put to be called")
	}
}

func TestIdempotencyMiddleware_CacheGetError(t *testing.T) {
	t.Parallel()

	cache := &fakeIdempotencyCache{getErr: true}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(TenantContextKey, uuid.New().String())
		return c.Next()
	})
	app.Use(NewIdempotencyMiddleware(cache))
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	req.Header.Set("X-Idempotency-Key", uuid.New().String())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
}

func TestIdempotencyMiddleware_EmptyTenant(t *testing.T) {
	t.Parallel()

	cache := &fakeIdempotencyCache{}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(NewIdempotencyMiddleware(cache))
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		return c.Status(201).JSON(fiber.Map{"id": 10})
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	req.Header.Set("X-Idempotency-Key", uuid.New().String())
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("want 201, got %d", resp.StatusCode)
	}
}

func TestIdempotencyMiddleware_Non200Response(t *testing.T) {
	t.Parallel()

	cache := &fakeIdempotencyCache{}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(TenantContextKey, uuid.New().String())
		return c.Next()
	})
	app.Use(NewIdempotencyMiddleware(cache))
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		return c.Status(500).JSON(fiber.Map{"error": "internal"})
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	req.Header.Set("X-Idempotency-Key", uuid.New().String())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("want 500, got %d", resp.StatusCode)
	}
	// Cache should NOT be called for non-200 responses
	if cache.putCalled > 0 {
		t.Fatal("cache.Put should not be called for non-200 responses")
	}
}

type fakeIdempotencyCache struct {
	getResult  json.RawMessage
	getOK      bool
	getErr     bool
	putCalled  int
	putLastKey string
}

func (f *fakeIdempotencyCache) Get(_ context.Context, _, _ string) (json.RawMessage, bool, error) {
	if f.getErr {
		return nil, false, context.DeadlineExceeded
	}
	return f.getResult, f.getOK, nil
}

func (f *fakeIdempotencyCache) Put(_ context.Context, _, key string, resp json.RawMessage) error {
	f.putCalled++
	f.putLastKey = key
	return nil
}

func TestLogMetricsScrapeAuthState(t *testing.T) {
	t.Parallel()
	// Should not panic
	LogMetricsScrapeAuthState("")
	LogMetricsScrapeAuthState("secret-token")
}

func TestMetricsScrapeAuth_NoToken(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MetricsScrapeAuth(""))
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.SendString("metrics")
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestMetricsScrapeAuth_ValidToken(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MetricsScrapeAuth("secret"))
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.SendString("metrics")
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestMetricsScrapeAuth_InvalidToken(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MetricsScrapeAuth("secret"))
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.SendString("metrics")
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestMetricsScrapeAuth_MissingHeader(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MetricsScrapeAuth("secret"))
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.SendString("metrics")
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestMetricsScrapeAuth_WrongPrefix(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(MetricsScrapeAuth("secret"))
	app.Get("/metrics", func(c *fiber.Ctx) error {
		return c.SendString("metrics")
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Basic secret")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestRequireWriteRole_ReadOnlyRejected(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(RoleContextKey, "readonly")
		return c.Next()
	})
	app.Post("/v1/memories", RequireWriteRole, func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/v1/memories", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("want 403, got %d", resp.StatusCode)
	}
}

func TestRequireWriteRole_OtherRolesAllowed(t *testing.T) {
	t.Parallel()

	roles := []string{"admin", "user", "writer", ""}
	for _, role := range roles {
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		if role != "" {
			app.Use(func(c *fiber.Ctx) error {
				c.Locals(RoleContextKey, role)
				return c.Next()
			})
		}
		app.Post("/v1/memories", RequireWriteRole, func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest("POST", "/v1/memories", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("role=%q: want 200, got %d", role, resp.StatusCode)
		}
	}
}

func TestRequireAdminRole_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("no role", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Get("/admin", RequireAdminRole, func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		resp, _ := app.Test(req)
		defer resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("want 403, got %d", resp.StatusCode)
		}
	})

	t.Run("user role rejected", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(RoleContextKey, "user")
			return c.Next()
		})
		app.Get("/admin", RequireAdminRole, func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		resp, _ := app.Test(req)
		defer resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("want 403, got %d", resp.StatusCode)
		}
	})

	t.Run("admin allowed", func(t *testing.T) {
		t.Parallel()
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		app.Use(func(c *fiber.Ctx) error {
			c.Locals(RoleContextKey, "admin")
			return c.Next()
		})
		app.Get("/admin", RequireAdminRole, func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequest("GET", "/admin", nil)
		resp, _ := app.Test(req)
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
	})
}

func TestIdempotencyKeyHeaderConst(t *testing.T) {
	if IdempotencyKeyHeader != "X-Idempotency-Key" {
		t.Fatal("IdempotencyKeyHeader mismatch")
	}
	if IdempotencyReplayedHeader != "X-Idempotency-Replayed" {
		t.Fatal("IdempotencyReplayedHeader mismatch")
	}
}
