package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/marco-spagn/pcmi/internal/model"
)

type stubAudit struct {
	entries []model.AuditEntry
	total   int
	err     error
}

func (s *stubAudit) List(_ context.Context, _ string, _, _ int, _ *time.Time) ([]model.AuditEntry, int, error) {
	return s.entries, s.total, s.err
}

func TestAuditHandlerList_invalidSince(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &AuditHandler{repo: &stubAudit{}}
	app.Get("/audit", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/audit?since=not-rfc3339", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestAuditHandlerList_repositoryError(t *testing.T) {
	t.Parallel()
	app := newTestApp(uuid.New().String(), "admin")
	h := &AuditHandler{repo: &stubAudit{err: errors.New("db")}}
	app.Get("/audit", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/audit", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
}

func TestAuditHandlerList_success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New().String()
	when := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	entries := []model.AuditEntry{
		{ID: 1, TenantID: tenantID, EventType: "read", Path: "/x", Method: "GET", StatusCode: 200, CreatedAt: when},
	}
	app := newTestApp(tenantID, "admin")
	h := &AuditHandler{repo: &stubAudit{entries: entries, total: 100}}
	app.Get("/audit", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/audit?limit=10&offset=5&since=2024-01-01T00:00:00Z", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Entries []model.AuditEntry `json:"entries"`
		Total   int                `json:"total"`
		Limit   int                `json:"limit"`
		Offset  int                `json:"offset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 100 || body.Limit != 10 || body.Offset != 5 || len(body.Entries) != 1 {
		t.Fatalf("unexpected %+v", body)
	}
}
