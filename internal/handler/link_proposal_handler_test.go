package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestLinkProposalHandler_Generate_Disabled(t *testing.T) {
	cfg := &config.Config{LinkProposalsEnabled: false}
	h := newLinkProposalHandler(nil, nil, nil, cfg)
	app := newTestApp("tid", "write")
	app.Post("/v1/graph/link-proposals/generate/:memory_id", h.Generate)

	resp, err := app.Test(httptest.NewRequest("POST", "/v1/graph/link-proposals/generate/1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status %d want 503", resp.StatusCode)
	}
}

func TestLinkProposalHandler_List_InvalidSourceID(t *testing.T) {
	cfg := &config.Config{}
	h := newLinkProposalHandler(nil, nil, nil, cfg)
	app := newTestApp("tid", "user")
	app.Get("/v1/graph/link-proposals", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/link-proposals?source_memory_id=abc", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestLinkProposalHandler_Accept_InvalidID(t *testing.T) {
	cfg := &config.Config{}
	h := newLinkProposalHandler(nil, nil, nil, cfg)
	app := newTestApp("tid", "write")
	app.Post("/v1/graph/link-proposals/:id/accept", h.Accept)

	resp, err := app.Test(httptest.NewRequest("POST", "/v1/graph/link-proposals/x/accept", strings.NewReader("")))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}
