package extraction

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

const MetadataKey = "pcmi_extract"

// SlotDef describes one attribute slot in a domain profile.
type SlotDef struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Values   []string `json:"values,omitempty"`
	Nullable bool     `json:"nullable"`
}

// EntityPromotion maps a slot to a future graph vertex label (Phase B).
type EntityPromotion struct {
	VertexLabel string `json:"vertex_label"`
	Normalize   string `json:"normalize"`
}

// Profile is a tenant-defined extraction schema.
type Profile struct {
	ProfileID        string                     `json:"profile_id"`
	Version          int                        `json:"version"`
	Description      string                     `json:"description,omitempty"`
	RequiredSlots    []SlotDef                  `json:"required_slots"`
	EntityPromotion  map[string]EntityPromotion `json:"entity_promotion,omitempty"`
	LinkProposalHints []string                  `json:"link_proposal_hints,omitempty"`
}

// EvidenceSpan grounds a slot value in source text (optional).
type EvidenceSpan struct {
	Slot  string `json:"slot"`
	Quote string `json:"quote"`
	Start int    `json:"start,omitempty"`
	End   int    `json:"end,omitempty"`
}

// Record is persisted under memory metadata key pcmi_extract.
type Record struct {
	ProfileID      string                 `json:"profile_id"`
	ProfileVersion int                    `json:"profile_version"`
	MemoryID       int64                  `json:"memory_id"`
	MemoryVersion  int                    `json:"memory_version"`
	ExtractedAt    string                 `json:"extracted_at"`
	Model          string                 `json:"model,omitempty"`
	Confidence     float64                `json:"confidence"`
	Slots          map[string]interface{} `json:"slots"`
	EvidenceSpans  []EvidenceSpan         `json:"evidence_spans,omitempty"`
	Status         string                 `json:"status,omitempty"` // ok | validation_failed | llm_failed
	Error          string                 `json:"error,omitempty"`
}

// LLMResponse is the JSON shape requested from the model.
type LLMResponse struct {
	Confidence    float64                `json:"confidence"`
	Slots         map[string]interface{} `json:"slots"`
	EvidenceSpans []EvidenceSpan         `json:"evidence_spans,omitempty"`
}

var hostnameRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-_.]{0,253}[a-zA-Z0-9])?$`)

// ParseProfile unmarshals and validates a profile document.
func ParseProfile(raw json.RawMessage) (*Profile, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("profile body is required")
	}
	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("invalid profile json: %w", err)
	}
	return ValidateProfile(&p)
}

// ValidateProfile checks invariants on an already-unmarshaled profile.
func ValidateProfile(p *Profile) (*Profile, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}
	p.ProfileID = strings.TrimSpace(p.ProfileID)
	if p.ProfileID == "" {
		return nil, fmt.Errorf("profile_id is required")
	}
	if p.Version < 1 {
		p.Version = 1
	}
	if len(p.RequiredSlots) == 0 {
		return nil, fmt.Errorf("required_slots must not be empty")
	}
	seen := make(map[string]struct{}, len(p.RequiredSlots))
	for i, slot := range p.RequiredSlots {
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			return nil, fmt.Errorf("required_slots[%d].name is required", i)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate slot name %q", name)
		}
		seen[name] = struct{}{}
		p.RequiredSlots[i].Name = name
		typ := strings.TrimSpace(strings.ToLower(slot.Type))
		if typ == "" {
			typ = "string"
		}
		switch typ {
		case "string", "enum", "ip", "hostname", "datetime":
			p.RequiredSlots[i].Type = typ
		default:
			return nil, fmt.Errorf("unsupported slot type %q for %s", slot.Type, name)
		}
		if typ == "enum" && len(slot.Values) == 0 {
			return nil, fmt.Errorf("enum slot %s requires values", name)
		}
	}
	return p, nil
}

// BuildSystemPrompt renders instructions for the LLM from a profile.
func BuildSystemPrompt(p *Profile) string {
	var b strings.Builder
	b.WriteString("You extract structured attributes from a memory record.\n")
	b.WriteString("Return ONLY valid JSON matching this schema:\n")
	b.WriteString(`{"confidence":0.0-1.0,"slots":{...},"evidence_spans":[{"slot":"name","quote":"..."}]}` + "\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Include EVERY required slot key in slots; use null when unknown.\n")
	b.WriteString("- Do not invent values not supported by the text or metadata hints.\n")
	b.WriteString("- confidence reflects overall extraction certainty.\n")
	if p.Description != "" {
		b.WriteString("Domain: ")
		b.WriteString(p.Description)
		b.WriteByte('\n')
	}
	b.WriteString("Required slots:\n")
	for _, slot := range p.RequiredSlots {
		fmt.Fprintf(&b, "- %s (%s", slot.Name, slot.Type)
		if slot.Type == "enum" {
			b.WriteString(": ")
			b.WriteString(strings.Join(slot.Values, "|"))
		}
		if slot.Nullable {
			b.WriteString(", nullable")
		}
		b.WriteString(")\n")
	}
	return b.String()
}

// BuildUserMessage formats content and optional metadata hints for the model.
func BuildUserMessage(content string, metadata map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("Memory content:\n")
	b.WriteString(strings.TrimSpace(content))
	if len(metadata) > 0 {
		hints := make(map[string]interface{}, len(metadata))
		for k, v := range metadata {
			if k == MetadataKey {
				continue
			}
			hints[k] = v
		}
		if len(hints) > 0 {
			raw, _ := json.Marshal(hints)
			b.WriteString("\n\nExisting metadata hints (may be incomplete):\n")
			b.Write(raw)
		}
	}
	return b.String()
}

// ParseLLMResponse parses and validates model output against the profile.
func ParseLLMResponse(raw string, p *Profile) (*Record, error) {
	cleaned := sanitizeJSON(raw)
	var resp LLMResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("parse llm json: %w", err)
	}
	if resp.Slots == nil {
		resp.Slots = map[string]interface{}{}
	}
	slots, err := ValidateSlots(p, resp.Slots)
	if err != nil {
		return nil, err
	}
	conf := resp.Confidence
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}
	return &Record{
		ProfileID:      p.ProfileID,
		ProfileVersion: p.Version,
		Confidence:     conf,
		Slots:          slots,
		EvidenceSpans:  resp.EvidenceSpans,
		Status:         "ok",
	}, nil
}

// ValidateSlots ensures all required keys exist and values match slot types.
func ValidateSlots(p *Profile, slots map[string]interface{}) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(p.RequiredSlots))
	for _, slot := range p.RequiredSlots {
		val, ok := slots[slot.Name]
		if !ok {
			if slot.Nullable {
				out[slot.Name] = nil
				continue
			}
			return nil, fmt.Errorf("missing required slot %q", slot.Name)
		}
		if val == nil {
			if !slot.Nullable {
				return nil, fmt.Errorf("slot %q must not be null", slot.Name)
			}
			out[slot.Name] = nil
			continue
		}
		normalized, err := validateSlotValue(slot, val)
		if err != nil {
			return nil, err
		}
		out[slot.Name] = normalized
	}
	return out, nil
}

func validateSlotValue(slot SlotDef, val interface{}) (interface{}, error) {
	switch slot.Type {
	case "string", "enum":
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("slot %q must be a string", slot.Name)
		}
		s = strings.TrimSpace(s)
		if s == "" && !slot.Nullable {
			return nil, fmt.Errorf("slot %q must not be empty", slot.Name)
		}
		if slot.Type == "enum" && s != "" {
			for _, allowed := range slot.Values {
				if s == allowed {
					return s, nil
				}
			}
			return nil, fmt.Errorf("slot %q value %q not in enum", slot.Name, s)
		}
		return s, nil
	case "ip":
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("slot %q must be a string IP", slot.Name)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			if slot.Nullable {
				return nil, nil
			}
			return nil, fmt.Errorf("slot %q must not be empty", slot.Name)
		}
		if net.ParseIP(s) == nil {
			return nil, fmt.Errorf("slot %q invalid IP %q", slot.Name, s)
		}
		return s, nil
	case "hostname":
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("slot %q must be a string hostname", slot.Name)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			if slot.Nullable {
				return nil, nil
			}
			return nil, fmt.Errorf("slot %q must not be empty", slot.Name)
		}
		if len(s) > 255 || !hostnameRE.MatchString(s) {
			return nil, fmt.Errorf("slot %q invalid hostname %q", slot.Name, s)
		}
		return s, nil
	case "datetime":
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("slot %q must be an RFC3339 string", slot.Name)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			if slot.Nullable {
				return nil, nil
			}
			return nil, fmt.Errorf("slot %q must not be empty", slot.Name)
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			if _, err2 := time.Parse(time.RFC3339Nano, s); err2 != nil {
				return nil, fmt.Errorf("slot %q invalid datetime %q", slot.Name, s)
			}
		}
		return s, nil
	default:
		return val, nil
	}
}

var trailingCommaBeforeClose = regexp.MustCompile(`,(\s*[}\]])`)

func sanitizeJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "{"); i >= 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}
	s = trailingCommaBeforeClose.ReplaceAllString(s, "$1")
	return s
}

// RecordToMetadataMap wraps a Record for MergeCurrentMetadata.
func RecordToMetadataMap(rec *Record) map[string]interface{} {
	if rec == nil {
		return map[string]interface{}{}
	}
	raw, _ := json.Marshal(rec)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	return map[string]interface{}{MetadataKey: m}
}

// RecordFromMetadata reads pcmi_extract from memory metadata.
func RecordFromMetadata(metadata map[string]interface{}) (*Record, bool) {
	if metadata == nil {
		return nil, false
	}
	raw, ok := metadata[MetadataKey]
	if !ok || raw == nil {
		return nil, false
	}
	switch v := raw.(type) {
	case map[string]interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var rec Record
		if json.Unmarshal(b, &rec) != nil {
			return nil, false
		}
		return &rec, true
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var rec Record
		if json.Unmarshal(b, &rec) != nil {
			return nil, false
		}
		return &rec, true
	}
}

// ShouldSkipPath returns true for distilled/consolidated/internal paths.
func ShouldSkipPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return true
	}
	if strings.Contains(path, ".distilled") || strings.Contains(path, ".consolidated") {
		return true
	}
	return false
}
