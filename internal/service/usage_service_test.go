package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/usage"
)

type fakeUsageSource struct {
	rows     []repository.UsageRow
	err      error
	from, to time.Time
}

func (f *fakeUsageSource) Daily(_ context.Context, _ string, from, to time.Time) ([]repository.UsageRow, error) {
	f.from, f.to = from, to
	return f.rows, f.err
}

func usageRows() []repository.UsageRow {
	return []repository.UsageRow{
		{Day: "2026-09-01", Operation: "embedding", Provider: "openai", Model: "emb", Requests: 10, InputTokens: 1_000_000},
		{Day: "2026-09-01", Operation: "distillation", Provider: "openai", Model: "chat", Requests: 2, InputTokens: 1_000_000, OutputTokens: 1_000_000},
		{Day: "2026-09-02", Operation: "distillation", Provider: "anthropic", Model: "claude", Requests: 1, InputTokens: 10, OutputTokens: 10},
	}
}

func fixedNow() time.Time { return time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC) }

func TestUsageService_defaultWindowAndGrouping(t *testing.T) {
	src := &fakeUsageSource{rows: usageRows()}
	pricing := usage.Pricing{"emb": {InputPerMTok: 0.02}, "chat": {InputPerMTok: 1, OutputPerMTok: 2}}
	svc := NewUsageService(src, pricing)
	svc.now = fixedNow
	rep, err := svc.Report(context.Background(), "t", UsageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.From != "2026-08-26" || rep.To != "2026-09-24" || len(rep.GroupBy) != 2 {
		t.Fatalf("window/group: %+v", rep)
	}
	if len(rep.Rows) != 3 || rep.Rows[0].Operation != "distillation" || rep.Rows[0].Provider != "anthropic" {
		t.Fatalf("rows %+v", rep.Rows)
	}
	if rep.Rows[0].EstimatedCostUSD != nil {
		t.Fatal("claude is unpriced → null cost")
	}
	if c := rep.Rows[1].EstimatedCostUSD; c == nil || math.Abs(*c-3) > 1e-9 {
		t.Fatalf("chat cost %v", c)
	}
	if rep.Totals.Requests != 13 || rep.Totals.EstimatedCostUSD != nil || !rep.PricingConfigured {
		t.Fatalf("totals %+v", rep.Totals)
	}
	if len(rep.UnpricedModels) != 1 || rep.UnpricedModels[0] != "claude" {
		t.Fatalf("unpriced %v", rep.UnpricedModels)
	}
}

func TestUsageService_groupByDayAndNone(t *testing.T) {
	src := &fakeUsageSource{rows: usageRows()[:2]}
	svc := NewUsageService(src, usage.Pricing{"*": {InputPerMTok: 1}})
	svc.now = fixedNow
	rep, err := svc.Report(context.Background(), "t", UsageQuery{GroupBy: "day, day", From: "2026-09-01", To: "2026-09-02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rows) != 1 || rep.Rows[0].Day != "2026-09-01" || rep.Rows[0].Model != "" || rep.Rows[0].Requests != 12 {
		t.Fatalf("rows %+v", rep.Rows)
	}
	if c := rep.Totals.EstimatedCostUSD; c == nil || math.Abs(*c-2) > 1e-9 {
		t.Fatalf("totals cost %v", c)
	}
	rep, err = svc.Report(context.Background(), "t", UsageQuery{GroupBy: "none"})
	if err != nil || len(rep.Rows) != 1 || rep.Rows[0].Operation != "" {
		t.Fatalf("none: %+v %v", rep.Rows, err)
	}
	if !src.to.Equal(time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("to %v", src.to)
	}
}

func TestUsageService_noPricing(t *testing.T) {
	svc := NewUsageService(&fakeUsageSource{rows: usageRows()}, usage.Pricing{})
	rep, err := svc.Report(context.Background(), "t", UsageQuery{})
	if err != nil || rep.PricingConfigured || rep.Totals.EstimatedCostUSD != nil || len(rep.UnpricedModels) != 3 {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	empty, err := NewUsageService(&fakeUsageSource{}, nil).Report(context.Background(), "t", UsageQuery{})
	if err != nil || len(empty.Rows) != 0 || empty.Totals.EstimatedCostUSD == nil || *empty.Totals.EstimatedCostUSD != 0 {
		t.Fatalf("empty report %+v", empty)
	}
}

func TestUsageService_invalidQueries(t *testing.T) {
	svc := NewUsageService(&fakeUsageSource{}, nil)
	svc.now = fixedNow
	for _, q := range []UsageQuery{
		{GroupBy: "tenant"}, {From: "2026/09/01"}, {To: "yesterday"},
		{From: "2026-09-10", To: "2026-09-01"}, {From: "2024-01-01", To: "2026-01-01"},
	} {
		if _, err := svc.Report(context.Background(), "t", q); !errors.Is(err, ErrInvalidUsageQuery) {
			t.Errorf("%+v: got %v", q, err)
		}
	}
	if _, err := NewUsageService(&fakeUsageSource{err: errors.New("db")}, nil).Report(context.Background(), "t", UsageQuery{}); err == nil || errors.Is(err, ErrInvalidUsageQuery) {
		t.Fatalf("db error: %v", err)
	}
}
