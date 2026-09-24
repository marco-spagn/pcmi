package usage

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Price is the USD cost per million tokens for one model.
type Price struct {
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
}

// Pricing maps a model name to its price. The key "*" is the fallback for
// models without an explicit entry. PCMI ships no built-in prices (they go
// stale); operators set LLM_PRICING to the rates they actually pay.
type Pricing map[string]Price

// ParsePricing parses the LLM_PRICING JSON object, e.g.
//
//	{"gpt-4o-mini":{"input_per_mtok":0.15,"output_per_mtok":0.6},
//	 "text-embedding-3-small":{"input_per_mtok":0.02}}
//
// An empty string yields an empty Pricing (costs are then reported as null).
func ParsePricing(raw string) (Pricing, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Pricing{}, nil
	}
	var p Pricing
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, fmt.Errorf("LLM_PRICING: invalid JSON: %w", err)
	}
	for model, price := range p {
		if strings.TrimSpace(model) == "" {
			return nil, fmt.Errorf("LLM_PRICING: empty model name")
		}
		for _, v := range []float64{price.InputPerMTok, price.OutputPerMTok} {
			if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("LLM_PRICING: invalid price for %q", model)
			}
		}
	}
	return p, nil
}

// Cost returns the estimated USD cost and whether a price was found for model.
func (p Pricing) Cost(model string, inputTokens, outputTokens int64) (float64, bool) {
	price, ok := p[model]
	if !ok {
		price, ok = p["*"]
	}
	if !ok {
		return 0, false
	}
	return (float64(inputTokens)*price.InputPerMTok + float64(outputTokens)*price.OutputPerMTok) / 1e6, true
}
