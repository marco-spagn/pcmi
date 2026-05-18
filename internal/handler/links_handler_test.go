package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLinksHandlerPost_invalidJSON(t *testing.T) {
	app := newTestApp("tid", "admin")
	h := NewLinksHandler(nil, nil)
	app.Post("/v1/memories/links", h.Post)

	req := httptest.NewRequest("POST", "/v1/memories/links", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}
