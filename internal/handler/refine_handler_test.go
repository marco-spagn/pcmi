package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

func TestRefineHandlerMissingPrefix(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tenant-1", "admin")
	rfh := NewRefineHandler()
	app.Post("/v1/memories/refine", middleware.RequireWriteRole, rfh.Post)

	req := httptest.NewRequest("POST", "/v1/memories/refine", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for missing path_prefix, got %d", resp.StatusCode)
	}
}

func TestRefineHandlerQueuesEvent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tenant-1", "admin")
	rfh := NewRefineHandler()
	app.Post("/v1/memories/refine", middleware.RequireWriteRole, rfh.Post)

	body := `{"path_prefix":"root.test"}`
	req := httptest.NewRequest("POST", "/v1/memories/refine", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRefineHandlerReadonlyBlocked(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tenant-1", "readonly")
	rfh := NewRefineHandler()
	app.Post("/v1/memories/refine", middleware.RequireWriteRole, rfh.Post)

	body := `{"path_prefix":"root.test"}`
	req := httptest.NewRequest("POST", "/v1/memories/refine", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for readonly role, got %d", resp.StatusCode)
	}
}

func TestRefineHandlerBadJSON(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	event.InitRedis(mr.Addr())

	app := newTestApp("tenant-1", "admin")
	rfh := NewRefineHandler()
	app.Post("/v1/memories/refine", middleware.RequireWriteRole, rfh.Post)

	req := httptest.NewRequest("POST", "/v1/memories/refine", strings.NewReader(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for bad JSON, got %d", resp.StatusCode)
	}
}
