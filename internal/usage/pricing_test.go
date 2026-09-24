package usage

import (
	"math"
	"testing"
)

func TestParsePricing(t *testing.T) {
	p, err := ParsePricing(`{"gpt-4o-mini":{"input_per_mtok":0.15,"output_per_mtok":0.6},"*":{"input_per_mtok":1}}`)
	if err != nil || len(p) != 2 {
		t.Fatalf("p=%v err=%v", p, err)
	}
	if empty, err := ParsePricing("  "); err != nil || len(empty) != 0 {
		t.Fatalf("empty: %v %v", empty, err)
	}
	for _, bad := range []string{`{`, `[]`, `{"":{"input_per_mtok":1}}`, `{"m":{"input_per_mtok":-1}}`} {
		if _, err := ParsePricing(bad); err == nil {
			t.Errorf("%s: expected error", bad)
		}
	}
}

func TestPricingCost(t *testing.T) {
	p := Pricing{"gpt-4o-mini": {InputPerMTok: 0.15, OutputPerMTok: 0.6}}
	c, ok := p.Cost("gpt-4o-mini", 1_000_000, 500_000)
	if !ok || math.Abs(c-0.45) > 1e-9 {
		t.Fatalf("cost=%v ok=%v", c, ok)
	}
	if _, ok := p.Cost("other", 1, 1); ok {
		t.Fatal("unknown model without fallback must be unpriced")
	}
	p["*"] = Price{InputPerMTok: 2}
	if c, ok := p.Cost("other", 500_000, 99); !ok || math.Abs(c-1.0) > 1e-9 {
		t.Fatalf("fallback cost=%v ok=%v", c, ok)
	}
}
