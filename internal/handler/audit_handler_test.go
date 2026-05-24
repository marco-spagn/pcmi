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
	page    model.PageResponse
	err     error
}

func (s *stubAudit) List(_ context.Context, _ string, _ model.PageRequest, _ *time.Time) ([]model.AuditEntry, model.PageResponse, error) {
	return s.entries, s.page, s.err
}

func (s *stubAudit) Count(_ context.Context, _ string, _ *time.Time) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if len(s.entries) > 0 {
		return len(s.entries), nil
	}
	return 0, nil
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
	h := &AuditHandler{repo: &stubAudit{
		entries: entries,
		page:    model.PageResponse{HasMore: true, NextCursor: "cursor-token"},
	}}
	app.Get("/audit", h.List)

	resp, err := app.Test(httptest.NewRequest("GET", "/audit?limit=10&since=2024-01-01T00:00:00Z", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Entries    []model.AuditEntry `json:"entries"`
		Total      int                `json:"total"`
		Limit      int                `json:"limit"`
		Offset     int                `json:"offset"`
		NextCursor string             `json:"next_cursor"`
		HasMore    bool               `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Limit != 10 || body.Total != 1 || body.Offset != 0 || !body.HasMore || body.NextCursor != "cursor-token" || len(body.Entries) != 1 {
		t.Fatalf("unexpected %+v", body)
	}
}
