package repository

import (
	"strings"
	"testing"
)

func TestTemporalClause(t *testing.T) {
	clause := temporalClause("$3")
	if !strings.Contains(clause, "$3::timestamptz") {
		t.Errorf("temporal clause should contain $3::timestamptz, got: %s", clause)
	}
	if !strings.Contains(clause, "valid_to IS NULL") {
		t.Errorf("temporal clause should reference valid_to, got: %s", clause)
	}
	if !strings.Contains(clause, "valid_from") {
		t.Errorf("temporal clause should reference valid_from, got: %s", clause)
	}
}

func TestScopeFilters(t *testing.T) {
	filters := scopeFilters("4", "5")
	if !strings.Contains(filters, "$4") {
		t.Errorf("scope filters should reference $4, got: %s", filters)
	}
	if !strings.Contains(filters, "$5") {
		t.Errorf("scope filters should reference $5, got: %s", filters)
	}
}

func TestTagFilters(t *testing.T) {
	filters := tagFilters("6", "7")
	if !strings.Contains(filters, "$6") {
		t.Errorf("tag filters should reference $6, got: %s", filters)
	}
	if !strings.Contains(filters, "$7") {
		t.Errorf("tag filters should reference $7, got: %s", filters)
	}
	if !strings.Contains(filters, "cardinality") {
		t.Errorf("tag filters should check cardinality, got: %s", filters)
	}
}

func TestDefaultScoringWeights(t *testing.T) {
	w := DefaultScoringWeights(true)
	if w.Semantic != 0.40 {
		t.Errorf("Semantic = %v, want 0.40", w.Semantic)
	}
	if w.Lexical != 0.30 {
		t.Errorf("Lexical = %v, want 0.30", w.Lexical)
	}
	if w.Importance != 0.15 {
		t.Errorf("Importance = %v, want 0.15", w.Importance)
	}
	if w.Recency != 0.15 {
		t.Errorf("Recency = %v, want 0.15", w.Recency)
	}
	if w.HalflifeDays != 30.0 {
		t.Errorf("HalflifeDays = %v, want 30.0", w.HalflifeDays)
	}
	if !w.DecayEnabled {
		t.Error("expected DecayEnabled=true")
	}
}

func TestDefaultScoringWeights_decayDisabled(t *testing.T) {
	w := DefaultScoringWeights(false)
	if w.DecayEnabled {
		t.Error("expected DecayEnabled=false")
	}
}

func TestLegacyHybridWeights(t *testing.T) {
	w := LegacyHybridWeights()
	if w.Semantic != 0.55 {
		t.Errorf("Semantic = %v, want 0.55", w.Semantic)
	}
	if w.Lexical != 0.45 {
		t.Errorf("Lexical = %v, want 0.45", w.Lexical)
	}
	if w.DecayEnabled {
		t.Error("expected DecayEnabled=false for legacy weights")
	}
}

func TestRecencyFactor(t *testing.T) {
	tests := []struct {
		name          string
		ageDays       float64
		halflifeDays  float64
		wantApprox    float64
	}{
		{"zero age", 0, 30, 1.0},
		{"halflife age", 30, 30, 0.5},
		{"zero halflife", 30, 0, 1.0},
		{"negative halflife", 30, -5, 1.0},
		{"negative age", -10, 30, 1.0}, // clamped to 0
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecencyFactor(tt.ageDays, tt.halflifeDays)
			// Compare with tolerance
			if got < tt.wantApprox-0.01 || got > tt.wantApprox+0.01 {
				t.Errorf("RecencyFactor(%v, %v) = %v, want ~%v", tt.ageDays, tt.halflifeDays, got, tt.wantApprox)
			}
		})
	}
}

func TestHybridScore(t *testing.T) {
	w := ScoringWeights{
		Semantic:   0.4,
		Lexical:    0.3,
		Importance: 0.15,
		Recency:    0.15,
		HalflifeDays: 30,
		DecayEnabled: true,
	}
	score := HybridScore(w, 0.8, 0.7, 0.5, 30)
	if score <= 0 {
		t.Errorf("HybridScore should be positive, got %v", score)
	}
}

func TestHybridScore_noDecay(t *testing.T) {
	w := ScoringWeights{
		Semantic:   0.4,
		Lexical:    0.3,
		Importance: 0.15,
		Recency:    0.15,
		HalflifeDays: 30,
		DecayEnabled: false,
	}
	score := HybridScore(w, 0.8, 0.7, 0.5, 30)
	if score <= 0 {
		t.Errorf("HybridScore should be positive, got %v", score)
	}
}

func TestHybridScoreSQL(t *testing.T) {
	w := DefaultScoringWeights(true)
	sql := hybridScoreSQL(w, "cosine_expr", "bm25_expr")
	if !strings.Contains(sql, "cosine_expr") {
		t.Errorf("expected cosine_expr in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "bm25_expr") {
		t.Errorf("expected bm25_expr in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "importance") {
		t.Errorf("expected importance in SQL, got: %s", sql)
	}
}

func TestHybridScoreSQL_noDecay(t *testing.T) {
	w := DefaultScoringWeights(false)
	sql := hybridScoreSQL(w, "c", "b")
	if sql == "" {
		t.Fatal("expected non-empty SQL")
	}
	// With decay disabled, recExpr should be "0"
	if !strings.Contains(sql, "0") {
		t.Errorf("expected recency contribution to be 0 when decay disabled, got: %s", sql)
	}
}
