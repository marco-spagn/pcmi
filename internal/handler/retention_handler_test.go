package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
)

type stubRetention struct {
	err     error
	counts  model.EraseCounts
	deleted *string
}

func (s *stubRetention) List(context.Context, string) ([]model.RetentionPolicy, error) {
	return []model.RetentionPolicy{{ID: 1, PathPrefix: "a"}}, s.err
}
func (s *stubRetention) Upsert(_ context.Context, tid string, r model.RetentionPolicyRequest) (model.RetentionPolicy, error) {
	return model.RetentionPolicy{TenantID: tid, PathPrefix: r.PathPrefix, MaxAgeDays: r.MaxAgeDays}, s.err
}
func (s *stubRetention) Delete(_ context.Context, _ string, p string) error {
	s.deleted = &p
	return s.err
}
func (s *stubRetention) Erase(context.Context, string, model.EraseRequest, string) (model.EraseCounts, []string, error) {
	return s.counts, nil, s.err
}
func (s *stubRetention) EraseGraphVertices(context.Context, string, []string) (int, error) {
	return 0, nil
}

func retentionApp(role string, st *stubRetention) *fiber.App {
	app := newTestApp(uuid.New().String(), role)
	h := &RetentionHandler{svc: service.NewRetentionService(st)}
	app.Get("/retention-policies", h.List)
	app.Put("/retention-policies", middleware.RequireAdminRole, h.Put)
	app.Delete("/retention-policies", middleware.RequireAdminRole, h.Delete)
	app.Post("/memories/erase", middleware.RequireAdminRole, h.Erase)
	return app
}

func TestRetentionHandler_routes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		role   string
		st     *stubRetention
		method string
		url    string
		body   string
		want   int
	}{
		{"list any role", "readonly", &stubRetention{}, "GET", "/retention-policies", "", 200},
		{"list error", "admin", &stubRetention{err: errors.New("db")}, "GET", "/retention-policies", "", 500},
		{"put ok", "admin", &stubRetention{}, "PUT", "/retention-policies", `{"path_prefix":"a.b","max_age_days":30}`, 200},
		{"put invalid", "admin", &stubRetention{}, "PUT", "/retention-policies", `{"path_prefix":"a.b"}`, 400},
		{"put bad json", "admin", &stubRetention{}, "PUT", "/retention-policies", `{`, 400},
		{"put non-admin", "user", &stubRetention{}, "PUT", "/retention-policies", `{"path_prefix":"a","max_age_days":1}`, 403},
		{"delete ok", "admin", &stubRetention{}, "DELETE", "/retention-policies?path_prefix=a.b", "", 200},
		{"delete tenant-wide", "admin", &stubRetention{}, "DELETE", "/retention-policies?path_prefix=", "", 200},
		{"delete missing param", "admin", &stubRetention{}, "DELETE", "/retention-policies", "", 400},
		{"delete not found", "admin", &stubRetention{err: repository.ErrRetentionPolicyNotFound}, "DELETE", "/retention-policies?path_prefix=a", "", 404},
		{"erase ok", "admin", &stubRetention{counts: model.EraseCounts{MemoryVersions: 3}}, "POST", "/memories/erase", `{"path_prefix":"users.alice"}`, 200},
		{"erase empty prefix", "admin", &stubRetention{}, "POST", "/memories/erase", `{"path_prefix":""}`, 400},
		{"erase bad json", "admin", &stubRetention{}, "POST", "/memories/erase", `nope`, 400},
		{"erase non-admin", "user", &stubRetention{}, "POST", "/memories/erase", `{"path_prefix":"a"}`, 403},
		{"erase db error", "admin", &stubRetention{err: errors.New("db")}, "POST", "/memories/erase", `{"path_prefix":"a"}`, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := retentionApp(tc.role, tc.st)
			req := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("status %d want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestRetentionHandler_eraseBody(t *testing.T) {
	t.Parallel()
	app := retentionApp("admin", &stubRetention{counts: model.EraseCounts{MemoryVersions: 3, Links: 1}})
	req := httptest.NewRequest("POST", "/memories/erase", strings.NewReader(`{"path_prefix":"users.alice","dry_run":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out model.EraseResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.DryRun || out.Counts.MemoryVersions != 3 || out.Counts.Links != 1 || out.PathPrefix != "users.alice" {
		t.Fatalf("got %+v", out)
	}
}
