package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

// ─── local mock repo for handler tests ───────────────────────────────────────

type handlerMockRepo struct {
	storeErr error
}

func (r *handlerMockRepo) Store(_ context.Context, req model.StoreRequest, _ string) (int64, int, *int64, error) {
	if r.storeErr != nil {
		return 0, 0, nil, r.storeErr
	}
	if req.Path == "fail" {
		return 0, 0, nil, errors.New("store failed: db down")
	}
	return 1, 1, nil, nil
}

func (r *handlerMockRepo) Retrieve(_ context.Context, req model.RetrieveRequest, _ string, _ []float32) ([]model.MemoryEntry, error) {
	return []model.MemoryEntry{{ID: 1, Path: req.PathPrefix}}, nil
}

func (r *handlerMockRepo) GetByPath(_ context.Context, _ string, path string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	if path == "exists" {
		return &model.MemoryEntry{ID: 1, Path: path}, nil
	}
	return nil, errors.New("memory not found")
}

func (r *handlerMockRepo) GetByIDResolveCurrent(_ context.Context, _ string, memoryID int64) (*model.MemoryEntry, int64, error) {
	if memoryID == 1 {
		return &model.MemoryEntry{ID: 1, Path: "exists"}, memoryID, nil
	}
	return nil, memoryID, errors.New("memory not found")
}

func (r *handlerMockRepo) GetHistoricalVersion(_ context.Context, _ string, _ string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	return nil, errors.New("no historical version")
}

func (r *handlerMockRepo) ExportMemories(_ context.Context, _ string, _ string, _ int, _ bool) ([]model.MemoryEntry, error) {
	return []model.MemoryEntry{{ID: 1, Path: "root.a"}}, nil
}

func (r *handlerMockRepo) CompactPathHistory(_ context.Context, _ string, _ string, _ int) (int, error) {
	return 0, nil
}

func (r *handlerMockRepo) UpdateImportance(_ context.Context, _, _ string, _ float64) error {
	return nil
}

func (r *handlerMockRepo) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	return model.DedupModeNone, nil
}
func (r *handlerMockRepo) FindCurrentByContentHash(context.Context, string, string) (*model.MemoryEntry, error) {
	return nil, nil
}
func (r *handlerMockRepo) MergeCurrentMetadata(context.Context, string, string, map[string]interface{}, []string) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (r *handlerMockRepo) UpsertDedupLink(context.Context, string, string, string) error {
	return nil
}

// ─── helper ──────────────────────────────────────────────────────────────────

func newHandlerApp(t *testing.T, tenantID string) (*fiber.App, *service.MemoryService) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	event.InitRedis(mr.Addr())

	repo := &handlerMockRepo{}
	svc := service.NewMemoryService(repo, nil)
	app := newTestApp(tenantID, "admin")
	return app, svc
}

func newHandlerAppWithRepo(t *testing.T, tenantID string, repo *handlerMockRepo) (*fiber.App, *service.MemoryService) {
	t.Helper()
	mr, _ := miniredis.Run()
	t.Cleanup(mr.Close)
	event.InitRedis(mr.Addr())
	svc := service.NewMemoryService(repo, nil)
	return newTestApp(tenantID, "admin"), svc
}

// ─── POST /v1/memories ───────────────────────────────────────────────────────

func TestStoreMemoryHandlerSuccess(t *testing.T) {
	app, svc := newHandlerApp(t, "tenant-1")
	api := app.Group("/v1")
	api.Post("/memories", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.StoreRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Store(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"id": result.Entry.ID, "status": "stored", "version": result.Version})
	})

	body := `{"path":"root.test.handler","content":"hello handler"}`
	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, data)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["status"] != "stored" {
		t.Fatalf("unexpected status: %v", out["status"])
	}
}

func TestStoreMemoryHandlerBadJSON(t *testing.T) {
	app, svc := newHandlerApp(t, "tenant-1")
	api := app.Group("/v1")
	api.Post("/memories", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
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

	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestStoreMemoryHandlerRepoError(t *testing.T) {
	repo := &handlerMockRepo{storeErr: errors.New("db down")}
	app, svc := newHandlerAppWithRepo(t, "tenant-1", repo)
	api := app.Group("/v1")
	api.Post("/memories", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
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

	body := `{"path":"root.test","content":"x"}`
	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 500 {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

// ─── GET /v1/memories/* ──────────────────────────────────────────────────────

func TestGetMemoryByPathFound(t *testing.T) {
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
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(entry)
	})

	req := httptest.NewRequest("GET", "/v1/memories/exists", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetMemoryByPathNotFound(t *testing.T) {
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
			if strings.Contains(err.Error(), "not found") {
				return c.Status(404).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(entry)
	})

	req := httptest.NewRequest("GET", "/v1/memories/does-not-exist", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// ─── POST /v1/retrieve ───────────────────────────────────────────────────────

func TestRetrieveHandlerSuccess(t *testing.T) {
	app, svc := newHandlerApp(t, "tenant-1")
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

	body := `{"path_prefix":"root.test"}`
	req := httptest.NewRequest("POST", "/v1/retrieve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRetrieveHandlerBadJSON(t *testing.T) {
	app, svc := newHandlerApp(t, "tenant-1")
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

	req := httptest.NewRequest("POST", "/v1/retrieve", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
