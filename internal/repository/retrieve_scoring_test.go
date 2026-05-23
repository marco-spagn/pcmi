package repository

import (
	"math"
	"strings"
	"testing"
)

func TestImportanceScore_HighImportance_RanksHigher(t *testing.T) {
	w := DefaultScoringWeights(false)
	high := HybridScore(w, 0.5, 0.5, 0.95, 0)
	low := HybridScore(w, 0.5, 0.5, 0.05, 0)
	if high <= low {
		t.Fatalf("high importance should rank higher: high=%v low=%v", high, low)
	}
}

func TestImportanceScore_LowImportance_RanksLower(t *testing.T) {
	w := DefaultScoringWeights(false)
	lowImp := HybridScore(w, 0.8, 0.8, 0.1, 0)
	highImp := HybridScore(w, 0.8, 0.8, 0.9, 0)
	if lowImp >= highImp {
		t.Fatalf("lower importance should reduce score: lowImp=%v highImp=%v", lowImp, highImp)
	}
}

func TestTemporalDecay_OldMemory_ScoresLower(t *testing.T) {
	w := DefaultScoringWeights(true)
	recent := HybridScore(w, 0, 0, 0.5, 1)
	old := HybridScore(w, 0, 0, 0.5, 90)
	if old >= recent {
		t.Fatalf("old memory should score lower: recent=%v old=%v", recent, old)
	}
}

func TestTemporalDecay_RecentMemory_ScoresHigher(t *testing.T) {
	w := DefaultScoringWeights(true)
	recent := HybridScore(w, 0.6, 0.4, 0.5, 0.5)
	stale := HybridScore(w, 0.6, 0.4, 0.5, 60)
	if recent <= stale {
		t.Fatalf("recent should beat stale: recent=%v stale=%v", recent, stale)
	}
}

func TestTemporalDecay_HalflifeConfig_AffectsDecay(t *testing.T) {
	shortHL := ScoringWeights{Recency: 0.15, HalflifeDays: 7, DecayEnabled: true}
	longHL := ScoringWeights{Recency: 0.15, HalflifeDays: 90, DecayEnabled: true}
	age := 14.0
	shortScore := HybridScore(shortHL, 0, 0, 0.5, age)
	longScore := HybridScore(longHL, 0, 0, 0.5, age)
	if shortScore >= longScore {
		t.Fatalf("shorter halflife should decay faster: short=%v long=%v", shortScore, longScore)
	}
	if math.Abs(RecencyFactor(age, 7)-0.25) > 0.01 {
		t.Fatalf("expected ~0.25 factor at 1 halflife, got %v", RecencyFactor(age, 7))
	}
}

func TestDecayDisabled_AllMemoriesEqualAge(t *testing.T) {
	wOn := DefaultScoringWeights(true)
	wOff := DefaultScoringWeights(false)
	recentOn := HybridScore(wOn, 0.5, 0.5, 0.5, 1)
	oldOn := HybridScore(wOn, 0.5, 0.5, 0.5, 100)
	recentOff := HybridScore(wOff, 0.5, 0.5, 0.5, 1)
	oldOff := HybridScore(wOff, 0.5, 0.5, 0.5, 100)
	if math.Abs(recentOn-oldOn) < 0.01 {
		t.Fatal("decay enabled should separate scores by age")
	}
	if math.Abs(recentOff-oldOff) > 1e-9 {
		t.Fatalf("decay disabled should ignore age: recent=%v old=%v", recentOff, oldOff)
	}
}

func TestImportanceEndpoint_UpdatesScore(t *testing.T) {
	w := DefaultScoringWeights(false)
	before := HybridScore(w, 0.7, 0.3, 0.5, 0)
	after := HybridScore(w, 0.7, 0.3, 0.95, 0)
	if after <= before {
		t.Fatalf("higher importance should increase fused score: before=%v after=%v", before, after)
	}
}

func TestHybridScoreSQL_containsComponents(t *testing.T) {
	w := DefaultScoringWeights(true)
	expr := hybridScoreSQL(w, `(1 - (embedding <=> $3::vector))`, `pcmi_bm25_rank(content_tsv, websearch_to_tsquery('english', $5))`)
	for _, sub := range []string{"importance", "EXP(-LN(2)", "last_accessed_at"} {
		if !strings.Contains(expr, sub) {
			t.Fatalf("expected %q in %s", sub, expr)
		}
	}
}

func TestRecencyFactor_atZeroAge(t *testing.T) {
	if got := RecencyFactor(0, 30); math.Abs(got-1) > 1e-9 {
		t.Fatalf("age 0 should yield 1, got %v", got)
	}
}

func TestDefaultScoringWeights_sumApproxOne(t *testing.T) {
	w := DefaultScoringWeights(true)
	sum := w.Semantic + w.Lexical + w.Importance + w.Recency
	if math.Abs(sum-1.0) > 1e-6 {
		t.Fatalf("weights should sum to 1, got %v", sum)
	}
}

// BenchmarkHybridScore measures in-process fusion cost (proxy for retrieve ranking).
func BenchmarkHybridScore(b *testing.B) {
	w := DefaultScoringWeights(true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HybridScore(w, 0.82, 0.41, 0.5, float64(i%30))
	}
}
