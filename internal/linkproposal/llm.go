package linkproposal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/marco-spagn/pcmi/internal/extraction"
	"github.com/marco-spagn/pcmi/internal/model"
)

const maxCandidateContent = 1200

// Candidate is a correlated memory offered to the LLM as a link target.
type Candidate struct {
	MemoryID   int64
	Path       string
	Content    string
	Extraction *extraction.Record
}

// LLMProposal is one directed edge suggestion from the model.
type LLMProposal struct {
	ToMemoryID int64   `json:"to_memory_id"`
	LinkType   string  `json:"link_type"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// LLMResponse is the JSON shape requested from the model.
type LLMResponse struct {
	Proposals []LLMProposal `json:"proposals"`
}

// BuildSystemPrompt renders link proposal instructions from a profile.
func BuildSystemPrompt(profile *extraction.Profile) string {
	var b strings.Builder
	b.WriteString("You propose typed links between memory records for an analyst review queue.\n")
	b.WriteString("Return ONLY valid JSON:\n")
	b.WriteString(`{"proposals":[{"to_memory_id":123,"link_type":"related","confidence":0.0-1.0,"reason":"..."}]}` + "\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Propose links ONLY when evidence supports a relationship.\n")
	b.WriteString("- link_type must be one of: causal, temporal, contradicts, supports, related.\n")
	b.WriteString("- Do not propose self-links or duplicate obvious edges.\n")
	b.WriteString("- Return an empty proposals array when nothing is justified.\n")
	if profile != nil && profile.Description != "" {
		b.WriteString("Domain: ")
		b.WriteString(profile.Description)
		b.WriteByte('\n')
	}
	if profile != nil && len(profile.LinkProposalHints) > 0 {
		b.WriteString("Hints:\n")
		for _, hint := range profile.LinkProposalHints {
			b.WriteString("- ")
			b.WriteString(hint)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// BuildUserMessage formats the source memory and candidate set for the model.
func BuildUserMessage(source *model.MemoryEntry, sourceRec *extraction.Record, candidates []Candidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Source memory id=%d path=%s\n", source.ID, source.Path)
	b.WriteString("Source content:\n")
	b.WriteString(truncate(strings.TrimSpace(source.Content), maxCandidateContent))
	if sourceRec != nil && sourceRec.Status == "ok" && len(sourceRec.Slots) > 0 {
		raw, _ := json.Marshal(sourceRec.Slots)
		b.WriteString("\n\nSource extracted slots:\n")
		b.Write(raw)
	}
	b.WriteString("\n\nCandidate memories (may share entities):\n")
	for _, c := range candidates {
		fmt.Fprintf(&b, "\n--- id=%d path=%s ---\n", c.MemoryID, c.Path)
		b.WriteString(truncate(strings.TrimSpace(c.Content), maxCandidateContent))
		if c.Extraction != nil && c.Extraction.Status == "ok" && len(c.Extraction.Slots) > 0 {
			raw, _ := json.Marshal(c.Extraction.Slots)
			b.WriteString("\nExtracted slots: ")
			b.Write(raw)
		}
	}
	return b.String()
}

// ParseLLMResponse validates model output and normalizes proposals.
func ParseLLMResponse(raw string, sourceID int64, allowed map[int64]struct{}) ([]LLMProposal, error) {
	cleaned := sanitizeJSON(raw)
	var resp LLMResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("parse llm json: %w", err)
	}
	if len(resp.Proposals) == 0 {
		return nil, nil
	}
	out := make([]LLMProposal, 0, len(resp.Proposals))
	seen := make(map[string]struct{})
	for _, p := range resp.Proposals {
		if p.ToMemoryID <= 0 || p.ToMemoryID == sourceID {
			continue
		}
		if allowed != nil {
			if _, ok := allowed[p.ToMemoryID]; !ok {
				continue
			}
		}
		linkType, err := model.NormalizeLinkType(p.LinkType)
		if err != nil {
			continue
		}
		reason := strings.TrimSpace(p.Reason)
		if reason == "" {
			continue
		}
		conf := p.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}
		key := fmt.Sprintf("%d:%s", p.ToMemoryID, linkType)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, LLMProposal{
			ToMemoryID: p.ToMemoryID,
			LinkType:   linkType,
			Confidence: conf,
			Reason:     reason,
		})
	}
	return out, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func sanitizeJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "{"); i >= 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}
	return s
}
