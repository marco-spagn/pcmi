package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/marco-spagn/pcmi/internal/repository"
	"github.com/marco-spagn/pcmi/internal/usage"
)

// UsageSource reads daily usage buckets.
type UsageSource interface {
	Daily(ctx context.Context, tenantID string, from, to time.Time) ([]repository.UsageRow, error)
}

// Usage query limits.
const (
	UsageDefaultDays = 30
	UsageMaxDays     = 366
)

// ErrInvalidUsageQuery is returned for a bad from/to/group_by (HTTP 400).
var ErrInvalidUsageQuery = errors.New("invalid usage query")

// UsageQuery selects the reporting window and grouping.
type UsageQuery struct {
	From    string // YYYY-MM-DD (UTC); default To - 29 days
	To      string // YYYY-MM-DD (UTC); default today
	GroupBy string // comma list of day|operation|model; "" = operation,model; "none" = totals only
}

// UsageLine is one grouped row. Dimensions not in group_by are empty.
type UsageLine struct {
	Day              string   `json:"day,omitempty"`
	Operation        string   `json:"operation,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	Model            string   `json:"model,omitempty"`
	Requests         int64    `json:"requests"`
	InputTokens      int64    `json:"input_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	EstimatedCostUSD *float64 `json:"estimated_cost_usd"`
}

// UsageReport is the response of GET /v1/stats/usage.
type UsageReport struct {
	TenantID          string      `json:"tenant_id"`
	From              string      `json:"from"`
	To                string      `json:"to"`
	GroupBy           []string    `json:"group_by"`
	Rows              []UsageLine `json:"rows"`
	Totals            UsageLine   `json:"totals"`
	PricingConfigured bool        `json:"pricing_configured"`
	UnpricedModels    []string    `json:"unpriced_models,omitempty"`
}

// UsageService aggregates metered token usage and prices it with LLM_PRICING.
type UsageService struct {
	src     UsageSource
	pricing usage.Pricing
	now     func() time.Time
}

func NewUsageService(src UsageSource, pricing usage.Pricing) *UsageService {
	return &UsageService{src: src, pricing: pricing, now: time.Now}
}

func parseGroupBy(raw string) ([]string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "":
		return []string{"operation", "model"}, nil
	case "none":
		return []string{}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, g := range strings.Split(raw, ",") {
		g = strings.TrimSpace(g)
		switch g {
		case "day", "operation", "model":
			if !seen[g] {
				seen[g] = true
				out = append(out, g)
			}
		default:
			return nil, fmt.Errorf("%w: group_by %q (allowed: day, operation, model, none)", ErrInvalidUsageQuery, g)
		}
	}
	return out, nil
}

func (s *UsageService) window(q UsageQuery) (time.Time, time.Time, error) {
	to := s.now().UTC().Truncate(24 * time.Hour)
	if q.To != "" {
		t, err := time.Parse(time.DateOnly, q.To)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: to must be YYYY-MM-DD", ErrInvalidUsageQuery)
		}
		to = t
	}
	from := to.AddDate(0, 0, -(UsageDefaultDays - 1))
	if q.From != "" {
		f, err := time.Parse(time.DateOnly, q.From)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: from must be YYYY-MM-DD", ErrInvalidUsageQuery)
		}
		from = f
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from is after to", ErrInvalidUsageQuery)
	}
	if to.Sub(from) >= UsageMaxDays*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: window longer than %d days", ErrInvalidUsageQuery, UsageMaxDays)
	}
	return from, to, nil
}

// Report returns grouped usage with estimated cost. Cost is computed per
// model before grouping, so rows mixing models are priced correctly; a group
// containing any unpriced model reports estimated_cost_usd = null.
func (s *UsageService) Report(ctx context.Context, tenantID string, q UsageQuery) (UsageReport, error) {
	groupBy, err := parseGroupBy(q.GroupBy)
	if err != nil {
		return UsageReport{}, err
	}
	from, to, err := s.window(q)
	if err != nil {
		return UsageReport{}, err
	}
	raw, err := s.src.Daily(ctx, tenantID, from, to)
	if err != nil {
		return UsageReport{}, err
	}

	has := map[string]bool{}
	for _, g := range groupBy {
		has[g] = true
	}
	type acc struct {
		line     UsageLine
		cost     float64
		unpriced bool
	}
	groups := map[UsageLine]*acc{}
	var order []UsageLine
	total := &acc{}
	unpriced := map[string]bool{}

	for _, r := range raw {
		key := UsageLine{}
		if has["day"] {
			key.Day = r.Day
		}
		if has["operation"] {
			key.Operation = r.Operation
		}
		if has["model"] {
			key.Provider, key.Model = r.Provider, r.Model
		}
		g, ok := groups[key]
		if !ok {
			g = &acc{line: key}
			groups[key] = g
			order = append(order, key)
		}
		cost, priced := s.pricing.Cost(r.Model, r.InputTokens, r.OutputTokens)
		if !priced {
			unpriced[r.Model] = true
		}
		for _, a := range []*acc{g, total} {
			a.line.Requests += r.Requests
			a.line.InputTokens += r.InputTokens
			a.line.OutputTokens += r.OutputTokens
			a.cost += cost
			a.unpriced = a.unpriced || !priced
		}
	}

	finish := func(a *acc) UsageLine {
		l := a.line
		if !a.unpriced {
			c := a.cost
			l.EstimatedCostUSD = &c
		}
		return l
	}
	rows := make([]UsageLine, 0, len(order))
	for _, k := range order {
		rows = append(rows, finish(groups[k]))
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Model < b.Model
	})
	models := make([]string, 0, len(unpriced))
	for m := range unpriced {
		models = append(models, m)
	}
	sort.Strings(models)

	return UsageReport{
		TenantID:          tenantID,
		From:              from.Format(time.DateOnly),
		To:                to.Format(time.DateOnly),
		GroupBy:           groupBy,
		Rows:              rows,
		Totals:            finish(total),
		PricingConfigured: len(s.pricing) > 0,
		UnpricedModels:    models,
	}, nil
}
