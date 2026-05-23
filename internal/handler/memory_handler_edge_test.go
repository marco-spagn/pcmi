package handler

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

func TestStoreMemoryHandler_EmptyPathStillCallsService(t *testing.T) {
	app, svc := newHandlerApp(t, "tenant-1")
	api := app.Group("/v1")
	api.Post("/memories", func(c *fiber.Ctx) error {
		var req model.StoreRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Store(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"id": result.Entry.ID})
	})

	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(`{"path":"","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, data)
	}
}

func TestStoreMemoryHandler_PathFailReturns500(t *testing.T) {
	app, svc := newHandlerApp(t, "tenant-1")
	api := app.Group("/v1")
	api.Post("/memories", func(c *fiber.Ctx) error {
		var req model.StoreRequest
		_ = c.BodyParser(&req)
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		_, err := svc.Store(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(`{"path":"fail","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}
}

func TestGetMemoryByPath_EmptyPathParam(t *testing.T) {
	app, svc := newHandlerApp(t, "tenant-1")
	api := app.Group("/v1")
	api.Get("/memories/*", func(c *fiber.Ctx) error {
		path := strings.TrimPrefix(c.Params("*"), "/")
		if path == "" {
			return c.Status(400).JSON(fiber.Map{"error": "path is required"})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		entry, err := svc.GetByPath(c.Context(), tenantID, path, nil, nil)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(entry)
	})

	req := httptest.NewRequest("GET", "/v1/memories/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d want 400 for empty path", resp.StatusCode)
	}
}

type retrieveFailRepo struct {
	handlerMockRepo
}

func (r *retrieveFailRepo) Retrieve(_ context.Context, _ model.RetrieveRequest, _ string, _ []float32) ([]model.MemoryEntry, error) {
	return nil, errors.New("retrieve boom")
}

func newHandlerAppWithRetrieveRepo(t *testing.T, tenantID string, repo *retrieveFailRepo) (*fiber.App, *service.MemoryService) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	event.InitRedis(mr.Addr())
	svc := service.NewMemoryService(repo, nil)
	return newTestApp(tenantID, "admin"), svc
}

func TestRetrieveHandler_ServiceError500(t *testing.T) {
	repo := &retrieveFailRepo{}
	app, svc := newHandlerAppWithRetrieveRepo(t, "tenant-1", repo)
	api := app.Group("/v1")
	api.Post("/retrieve", func(c *fiber.Ctx) error {
		var req model.RetrieveRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Retrieve(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	req := httptest.NewRequest("POST", "/v1/retrieve", strings.NewReader(`{"path_prefix":"root"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}
}
