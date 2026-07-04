package entityalias

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
)

// CandidateEntity is a known canonical entity for alias matching.
type CandidateEntity struct {
	ID           string
	Kind         string
	CanonicalKey string
	DisplayName  string
}

// Proposal is one LLM-suggested alias merge.
type Proposal struct {
	AliasKey       string  `json:"alias_key"`
	TargetEntityID string  `json:"target_entity_id"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
}

type llmResponse struct {
	Proposals []Proposal `json:"proposals"`
}

// BuildSystemPrompt instructs the model to propose entity alias merges.
func BuildSystemPrompt(profile *extraction.Profile) string {
	var b strings.Builder
	b.WriteString("You identify when two extracted entity labels refer to the same real-world entity.\n")
	b.WriteString("Return ONLY JSON: {\"proposals\":[{\"alias_key\":\"...\",\"target_entity_id\":\"uuid\",\"confidence\":0.0-1.0,\"reason\":\"...\"}]}\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Propose only when confident the labels are synonymous (vendor naming, abbreviations, transliterations).\n")
	b.WriteString("- alias_key must be a normalized label from the source extraction (not the canonical target key).\n")
	b.WriteString("- target_entity_id must be one of the provided candidate entity UUIDs.\n")
	b.WriteString("- Return an empty proposals array when unsure.\n")
	if profile != nil && profile.Description != "" {
		b.WriteString("Domain: ")
		b.WriteString(profile.Description)
		b.WriteByte('\n')
	}
	return b.String()
}

// BuildUserMessage formats source extraction and candidate entities.
func BuildUserMessage(sourceKind, sourceKey string, sourceEntityID string, candidates []CandidateEntity) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Source entity kind=%s key=%q id=%s\n\nCandidates (same kind):\n", sourceKind, sourceKey, sourceEntityID)
	for _, c := range candidates {
		fmt.Fprintf(&b, "- id=%s canonical_key=%q display_name=%q\n", c.ID, c.CanonicalKey, c.DisplayName)
	}
	return b.String()
}

// ParseLLMResponse validates model output.
func ParseLLMResponse(raw, sourceEntityID string, allowed map[string]struct{}) ([]Proposal, error) {
	cleaned := strings.TrimSpace(raw)
	if i := strings.Index(cleaned, "{"); i >= 0 {
		cleaned = cleaned[i:]
	}
	if j := strings.LastIndex(cleaned, "}"); j >= 0 {
		cleaned = cleaned[:j+1]
	}
	var resp llmResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("parse entity alias proposals: %w", err)
	}
	out := make([]Proposal, 0, len(resp.Proposals))
	for _, p := range resp.Proposals {
		p.AliasKey = strings.TrimSpace(p.AliasKey)
		p.TargetEntityID = strings.TrimSpace(p.TargetEntityID)
		p.Reason = strings.TrimSpace(p.Reason)
		if p.AliasKey == "" || p.TargetEntityID == "" {
			continue
		}
		if p.TargetEntityID == sourceEntityID {
			continue
		}
		if _, ok := allowed[p.TargetEntityID]; !ok {
			continue
		}
		if p.Confidence < 0 {
			p.Confidence = 0
		}
		if p.Confidence > 1 {
			p.Confidence = 1
		}
		out = append(out, p)
	}
	return out, nil
}

// FromRegistryRows converts model rows to candidates, excluding one id.
func FromRegistryRows(rows []model.EntityRegistry, excludeID string) []CandidateEntity {
	out := make([]CandidateEntity, 0, len(rows))
	for _, r := range rows {
		if r.ID == excludeID {
			continue
		}
		out = append(out, CandidateEntity{
			ID:           r.ID,
			Kind:         r.Kind,
			CanonicalKey: r.CanonicalKey,
			DisplayName:  r.DisplayName,
		})
	}
	return out
}
