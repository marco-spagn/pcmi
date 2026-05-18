package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marco-spagn/pcmi/internal/service"
)

func TestBatchStoreBadJSON(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	registerBatchRoutes(app.Group("/v1"), service.NewMemoryService(nil, nil))

	req := httptest.NewRequest("POST", "/v1/memories/batch", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestBatchRetrieveBadJSON(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	registerBatchRoutes(app.Group("/v1"), service.NewMemoryService(nil, nil))

	req := httptest.NewRequest("POST", "/v1/retrieve/batch", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestMemoryExportBadJSON(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	registerBatchRoutes(app.Group("/v1"), service.NewMemoryService(nil, nil))

	req := httptest.NewRequest("POST", "/v1/memories/export", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestMemoryImportBadJSON(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	registerBatchRoutes(app.Group("/v1"), service.NewMemoryService(nil, nil))

	req := httptest.NewRequest("POST", "/v1/memories/import", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}
