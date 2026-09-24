package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/service"
	"github.com/marco-spagn/pcmi/internal/usage"
)

type stubUsage struct {
	rows []repository.UsageRow
	err  error
}

func (s *stubUsage) Daily(context.Context, string, time.Time, time.Time) ([]repository.UsageRow, error) {
	return s.rows, s.err
}

func TestUsageHandler(t *testing.T) {
	t.Parallel()
	rows := []repository.UsageRow{{Day: "2026-09-01", Operation: "embedding", Provider: "openai", Model: "emb", Requests: 3, InputTokens: 2_000_000}}
	for _, tc := range []struct {
		name  string
		src   *stubUsage
		query string
		want  int
	}{
		{"ok", &stubUsage{rows: rows}, "?from=2026-09-01&to=2026-09-02", 200},
		{"bad group", &stubUsage{}, "?group_by=tenant", 400},
		{"bad date", &stubUsage{}, "?from=nope", 400},
		{"db error", &stubUsage{err: errors.New("db")}, "", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(uuid.New().String(), "readonly")
			h := &UsageHandler{svc: service.NewUsageService(tc.src, usage.Pricing{"emb": {InputPerMTok: 0.5}})}
			app.Get("/stats/usage", h.Get)
			resp, err := app.Test(httptest.NewRequest("GET", "/stats/usage"+tc.query, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("status %d want %d", resp.StatusCode, tc.want)
			}
			if tc.want == 200 {
				var rep service.UsageReport
				if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
					t.Fatal(err)
				}
				if rep.Totals.EstimatedCostUSD == nil || *rep.Totals.EstimatedCostUSD != 1 || rep.Totals.Requests != 3 {
					t.Fatalf("report %+v", rep.Totals)
				}
			}
		})
	}
}
