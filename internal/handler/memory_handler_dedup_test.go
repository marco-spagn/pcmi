package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

type dedupHandlerMockRepo struct {
	mu         sync.Mutex
	entries    map[string]*model.MemoryEntry
	links      int
	storeCalls int
}

func (d *dedupHandlerMockRepo) seed(path, content string, id int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.entries == nil {
		d.entries = map[string]*model.MemoryEntry{}
	}
	d.entries[path] = &model.MemoryEntry{
		ID: id, Path: path, Content: content, Version: 1,
		Metadata: map[string]interface{}{},
	}
}

func (d *dedupHandlerMockRepo) Store(_ context.Context, req model.StoreRequest, _ string) (int64, int, *int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.storeCalls++
	id := int64(200 + d.storeCalls)
	d.entries[req.Path] = &model.MemoryEntry{
		ID: id, Path: req.Path, Content: req.Content, Version: 1,
	}
	return id, 1, nil, nil
}

func (d *dedupHandlerMockRepo) Retrieve(context.Context, model.RetrieveRequest, string, []float32) ([]model.MemoryEntry, error) {
	return nil, nil
}
func (d *dedupHandlerMockRepo) GetHistoricalVersion(context.Context, string, string, *int, *time.Time) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (d *dedupHandlerMockRepo) GetByPath(_ context.Context, _ string, path string, _ *int, _ *time.Time) (*model.MemoryEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.entries[path]; ok {
		cp := *e
		return &cp, nil
	}
	return nil, errors.New("not found")
}
func (d *dedupHandlerMockRepo) GetByIDResolveCurrent(_ context.Context, _ string, memoryID int64) (*model.MemoryEntry, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.entries {
		if e.ID == memoryID {
			cp := *e
			return &cp, memoryID, nil
		}
	}
	return nil, memoryID, errors.New("memory not found")
}
func (d *dedupHandlerMockRepo) ExportMemories(context.Context, string, string, int, bool) ([]model.MemoryEntry, error) {
	return nil, nil
}
func (d *dedupHandlerMockRepo) CompactPathHistory(context.Context, string, string, int) (int, error) {
	return 0, nil
}
func (d *dedupHandlerMockRepo) UpdateImportance(context.Context, string, string, float64) error {
	return nil
}
func (d *dedupHandlerMockRepo) GetTenantDedupMode(context.Context, string) (model.DedupMode, error) {
	return model.DedupModeNone, nil
}
func (d *dedupHandlerMockRepo) FindCurrentByContentHash(_ context.Context, _ string, hash string) (*model.MemoryEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.entries {
		if model.ContentHash(e.Content) == hash {
			cp := *e
			return &cp, nil
		}
	}
	return nil, nil
}
func (d *dedupHandlerMockRepo) MergeCurrentMetadata(context.Context, string, string, map[string]interface{}, []string) (*model.MemoryEntry, error) {
	return nil, errors.New("not implemented")
}
func (d *dedupHandlerMockRepo) UpsertDedupLink(context.Context, string, string, string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.links++
	return nil
}

func registerDedupStoreRoute(app *fiber.App, svc *service.MemoryService) {
	api := app.Group("/v1")
	api.Post("/memories", middleware.RequireWriteRole, func(c *fiber.Ctx) error {
		var req model.StoreRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if hdr := strings.TrimSpace(c.Get("X-Dedup-Mode")); hdr != "" && strings.TrimSpace(req.DedupMode) == "" {
			req.DedupMode = hdr
		}
		if req.DedupMode != "" {
			if _, err := model.ParseDedupMode(req.DedupMode); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Store(c.Context(), &req, tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		status := "stored"
		if result.Action != "" && result.Action != model.StoreActionStored {
			status = "deduplicated"
		}
		resp := fiber.Map{"id": result.Entry.ID, "status": status, "version": result.Version}
		if result.Action != "" && result.Action != model.StoreActionStored {
			resp["dedup_action"] = result.Action
		}
		if result.LinkedFrom != "" {
			resp["linked_from"] = result.LinkedFrom
		}
		return c.JSON(resp)
	})
}

func TestStoreMemoryHandler_DedupHeaderSkip(t *testing.T) {
	repo := &dedupHandlerMockRepo{}
	repo.seed("root.dedup", "same payload", 42)
	svc := service.NewMemoryService(repo, nil, model.DedupModeNone)
	app := newTestApp("tenant-1", "admin")
	registerDedupStoreRoute(app, svc)

	body := `{"path":"root.dedup","content":"same payload"}`
	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dedup-Mode", "skip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, data)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["dedup_action"] != model.StoreActionSkipped || out["status"] != "deduplicated" {
		t.Fatalf("unexpected response: %v", out)
	}
	if repo.storeCalls != 0 {
		t.Fatalf("expected no store, calls=%d", repo.storeCalls)
	}
}

func TestStoreMemoryHandler_DedupHeaderLink(t *testing.T) {
	repo := &dedupHandlerMockRepo{}
	repo.seed("root.canonical", "linked body", 7)
	svc := service.NewMemoryService(repo, nil, model.DedupModeNone)
	app := newTestApp("tenant-1", "admin")
	registerDedupStoreRoute(app, svc)

	body := `{"path":"root.alias","content":"linked body"}`
	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dedup-Mode", "link")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["dedup_action"] != model.StoreActionLinked || repo.storeCalls != 0 || repo.links != 1 {
		t.Fatalf("response=%v store=%d links=%d", out, repo.storeCalls, repo.links)
	}
}

func TestStoreMemoryHandler_InvalidDedupHeader(t *testing.T) {
	repo := &dedupHandlerMockRepo{}
	svc := service.NewMemoryService(repo, nil)
	app := newTestApp("tenant-1", "admin")
	registerDedupStoreRoute(app, svc)

	req := httptest.NewRequest("POST", "/v1/memories", strings.NewReader(`{"path":"root.x","content":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dedup-Mode", "invalid")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
