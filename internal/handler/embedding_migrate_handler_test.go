package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddingMigrateBadJSON(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	app.Post("/migrate", NewEmbeddingMigrateHandler(nil).Migrate)

	req := httptest.NewRequest("POST", "/migrate", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

func TestEmbeddingMigrateMissingPathPrefix(t *testing.T) {
	app := newTestApp("00000000-0000-0000-0000-000000000000", "admin")
	app.Post("/migrate", NewEmbeddingMigrateHandler(nil).Migrate)

	req := httptest.NewRequest("POST", "/migrate", strings.NewReader(`{"target_model":"m"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}
