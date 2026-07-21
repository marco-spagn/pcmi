package extraction

import "strings"

// PromotedEntity is a slot value materialized as an AGE :Entity vertex.
type PromotedEntity struct {
	Slot       string  `json:"slot"`
	Kind       string  `json:"kind"`
	Key        string  `json:"key"`
	Confidence float64 `json:"confidence"`
}

// PromoteEntities returns deterministic entity vertices from a validated record.
func PromoteEntities(profile *Profile, rec *Record) []PromotedEntity {
	if profile == nil || rec == nil || rec.Status != "ok" || len(profile.EntityPromotion) == 0 {
		return nil
	}
	if rec.Slots == nil {
		return nil
	}
	out := make([]PromotedEntity, 0, len(profile.EntityPromotion))
	for slot, promo := range profile.EntityPromotion {
		raw, ok := rec.Slots[slot]
		if !ok || raw == nil {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		key := NormalizeEntityKey(s, promo.Normalize)
		if key == "" {
			continue
		}
		kind := strings.TrimSpace(promo.VertexLabel)
		if kind == "" {
			continue
		}
		out = append(out, PromotedEntity{
			Slot:       slot,
			Kind:       kind,
			Key:        key,
			Confidence: rec.Confidence,
		})
	}
	return out
}

// NormalizeEntityKey applies profile promotion rules to a slot string value.
func NormalizeEntityKey(value, normalize string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(normalize)) {
	case "lower":
		return strings.ToLower(value)
	case "upper":
		return strings.ToUpper(value)
	case "trim", "":
		return value
	default:
		// alias_table: lookup handled by EntityRegistryService (Phase D); basic normalize here.
		return value
	}
}
