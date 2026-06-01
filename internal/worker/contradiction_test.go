package worker

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
)

func TestDetectContradiction_NoNegation(t *testing.T) {
	a := "The system runs correctly on all platforms with no known issues."
	b := "Performance benchmarks show good throughput numbers."
	contradicts, conf := detectContradiction(a, b)
	if contradicts {
		t.Errorf("no negation in either text, expected false; got confidence=%.2f", conf)
	}
}

func TestDetectContradiction_OneNegation_NoOverlap(t *testing.T) {
	a := "The system does not have any performance issues."
	b := "Pizza is a popular food in many countries around the world."
	contradicts, _ := detectContradiction(a, b)
	if contradicts {
		t.Error("no topic overlap between texts, expected false")
	}
}

func TestDetectContradiction_DirectContradiction(t *testing.T) {
	a := "The API does not support batch operations and never will."
	b := "The API supports batch operations for all endpoints."
	contradicts, conf := detectContradiction(a, b)
	if !contradicts {
		t.Error("direct contradiction between texts, expected true")
	}
	if conf < 0.30 {
		t.Errorf("confidence too low: %.2f < 0.30", conf)
	}
}

func TestDetectContradiction_NegationWithOverlap(t *testing.T) {
	a := "The memory system incorrectly stores encrypted data without proper key management."
	b := "The memory system correctly stores encrypted data with proper key management."
	contradicts, conf := detectContradiction(a, b)
	if !contradicts {
		t.Error("texts with negation + high overlap, expected contradiction")
	}
	if conf < 0.40 {
		t.Errorf("confidence too low: %.2f", conf)
	}
}

func TestDetectContradiction_ShortText(t *testing.T) {
	a := "Not good."
	b := "This is a longer text that explains why the system is not working correctly."
	contradicts, _ := detectContradiction(a, b)
	if contradicts {
		t.Error("one text too short (<20 chars), expected false")
	}
}

func TestDetectContradiction_Empty(t *testing.T) {
	contradicts, conf := detectContradiction("", "")
	if contradicts {
		t.Error("empty texts, expected false")
	}
	if conf != 0 {
		t.Errorf("empty texts, expected confidence 0, got %.2f", conf)
	}
}

func TestDetectContradiction_BothEmpty(t *testing.T) {
	contradicts, conf := detectContradiction("   ", "   ")
	if contradicts {
		t.Error("whitespace-only texts, expected false")
	}
	if conf != 0 {
		t.Errorf("whitespace-only, expected confidence 0, got %.2f", conf)
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("the quick brown fox jumps over the lazy dog")
	if len(tokens) < 4 {
		t.Errorf("expected at least 4 meaningful tokens, got %d: %v", len(tokens), tokens)
	}
	// "the", "a" should be filtered
	for _, tok := range tokens {
		if tok == "the" || tok == "a" {
			t.Errorf("stopword %q should be filtered", tok)
		}
	}
}

func TestTokenize_Empty(t *testing.T) {
	tokens := tokenize("")
	if len(tokens) != 0 {
		t.Errorf("empty input, expected 0 tokens, got %d", len(tokens))
	}
}

func TestWordOverlap_Identical(t *testing.T) {
	a := []string{"foo", "bar", "baz"}
	b := []string{"foo", "bar", "baz"}
	overlap := wordOverlap(a, b)
	if overlap != 1.0 {
		t.Errorf("identical sets, expected 1.0, got %.2f", overlap)
	}
}

func TestWordOverlap_Disjoint(t *testing.T) {
	a := []string{"foo", "bar"}
	b := []string{"baz", "qux"}
	overlap := wordOverlap(a, b)
	if overlap != 0.0 {
		t.Errorf("disjoint sets, expected 0.0, got %.2f", overlap)
	}
}

func TestWordOverlap_Partial(t *testing.T) {
	a := []string{"foo", "bar", "baz"}
	b := []string{"foo", "baz", "qux"}
	overlap := wordOverlap(a, b)
	// intersection: {foo, baz} = 2; union: {foo, bar, baz, qux} = 4; 2/4 = 0.5
	if overlap < 0.45 || overlap > 0.55 {
		t.Errorf("partial overlap, expected ~0.5, got %.2f", overlap)
	}
}

func TestWordOverlap_Empty(t *testing.T) {
	overlap := wordOverlap(nil, nil)
	if overlap != 0.0 {
		t.Errorf("empty sets, expected 0.0, got %.2f", overlap)
	}
}

func TestNewContradictionWorker(t *testing.T) {
	cfg := &config.Config{
		ContradictionDetectionEnabled:     true,
		ContradictionDetectionIntervalSecs: 120,
	}
	w := NewContradictionWorker(nil, cfg)
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
	if !w.cfg.ContradictionDetectionEnabled {
		t.Error("expected contradiction detection enabled")
	}
}

func TestNewContradictionWorker_Disabled(t *testing.T) {
	cfg := &config.Config{
		ContradictionDetectionEnabled:     false,
		ContradictionDetectionIntervalSecs: 60,
	}
	w := NewContradictionWorker(nil, cfg)
	if w == nil {
		t.Fatal("expected non-nil worker even when disabled (caller checks cfg)")
	}
}

// ─── detectContradiction extra scenarios ─────────────────────────────────────

func TestDetectContradiction_WrongKeyword(t *testing.T) {
	a := "The implementation is wrong and produces incorrect results every time."
	b := "The implementation is correct and produces accurate results every time."
	contradicts, conf := detectContradiction(a, b)
	if !contradicts {
		t.Error("'wrong' and 'incorrect' keywords + overlap = contradiction")
	}
	if conf < 0.30 {
		t.Errorf("confidence too low: %.2f", conf)
	}
}

func TestDetectContradiction_WasNotVsWas(t *testing.T) {
	a := "The bug was not present in version 2.3.1 according to the test report."
	b := "The bug was present in version 2.3.1 according to the test report."
	contradicts, conf := detectContradiction(a, b)
	if !contradicts {
		t.Error("'was not' vs 'was' = negation contradiction")
	}
	if conf < 0.30 {
		t.Errorf("confidence too low: %.2f", conf)
	}
}

func TestDetectContradiction_DoesNotVsDoes(t *testing.T) {
	a := "The cache layer does not improve latency for reads under 10KB."
	b := "The cache layer does improve latency for reads under 10KB."
	contradicts, _ := detectContradiction(a, b)
	if !contradicts {
		t.Error("'does not' vs 'does' = contradiction")
	}
}

func TestDetectContradiction_FalseKeyword(t *testing.T) {
	a := "The hypothesis that the memory leak is fixed is false."
	b := "The memory leak is fixed and the fix is confirmed by the test suite."
	contradicts, _ := detectContradiction(a, b)
	if !contradicts {
		t.Error("'false' keyword + high overlap = contradiction")
	}
}

func TestDetectContradiction_NoOverlapHighNegation(t *testing.T) {
	a := "This does not work and is never correct for database queries."
	b := "The weather is not looking good and it never rains in this city."
	contradicts, _ := detectContradiction(a, b)
	if contradicts {
		t.Error("both have negations but no topic overlap, expected false")
	}
}

func TestDetectContradiction_EdgeCase_ExactSameText(t *testing.T) {
	text := "The system does not handle edge cases correctly and is never reliable."
	contradicts, _ := detectContradiction(text, text)
	// Same text: high overlap + negation = technically a "contradiction with itself"
	// This is acceptable since the overlap check doesn't filter self-comparisons.
	// The caller (checkPrefix) handles dedup via memory IDs.
	if !contradicts {
		t.Log("same text: detectContradiction returned false (negation in same text)")
	}
}

func TestDetectContradiction_NegationPrefix(t *testing.T) {
	a := "The nosql database performs well under load with no noticeable latency."
	b := "The sql database performs poorly under load with noticeable latency."
	contradicts, _ := detectContradiction(a, b)
	// "nosql" contains "no" prefix which is not a true negation but keyword
	// matching may flag it. Either outcome is acceptable.
	t.Logf("nosql/sql contradiction result: %v (either outcome is acceptable)", contradicts)
}

// ─── tokenize extended ──────────────────────────────────────────────────────

func TestTokenize_Numbers(t *testing.T) {
	tokens := tokenize("version 2 3 1 is released on 2024")
	found := false
	for _, tok := range tokens {
		if tok == "2" || tok == "3" || tok == "2024" {
			found = true
		}
	}
	if !found {
		t.Error("numbers should be preserved in tokenization")
	}
}

func TestTokenize_OnlyStopwords(t *testing.T) {
	tokens := tokenize("the a an of to in and it that this was for on with as be by at or from are has been but not no its can will")
	if len(tokens) != 0 {
		t.Errorf("only stopwords: expected 0 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestTokenize_Punctuation(t *testing.T) {
	tokens := tokenize("hello, world! how are you?")
	if len(tokens) < 4 {
		t.Errorf("punctuation should be stripped: got %d tokens: %v", len(tokens), tokens)
	}
}

// ─── wordOverlap extended ────────────────────────────────────────────────────

func TestWordOverlap_OneEmpty(t *testing.T) {
	a := []string{"foo", "bar"}
	overlap := wordOverlap(a, nil)
	if overlap != 0.0 {
		t.Errorf("one empty set: expected 0, got %.2f", overlap)
	}
}

func TestWordOverlap_SingleElement(t *testing.T) {
	a := []string{"unique"}
	b := []string{"unique"}
	overlap := wordOverlap(a, b)
	if overlap != 1.0 {
		t.Errorf("single identical element: expected 1.0, got %.2f", overlap)
	}
}

func TestWordOverlap_DuplicatesInInput(t *testing.T) {
	a := []string{"foo", "foo", "bar", "bar"}
	b := []string{"foo", "bar", "baz"}
	overlap := wordOverlap(a, b)
	// Sets: a={foo,bar}, b={foo,bar,baz}; intersection=2, union=3
	if overlap < 0.6 || overlap > 0.7 {
		t.Errorf("duplicates should not affect overlap: got %.2f, want ~0.67", overlap)
	}
}

// ─── OnMemoryEvent edge cases ────────────────────────────────────────────────

func TestOnMemoryEvent_EmptyTenantID(t *testing.T) {
	cfg := &config.Config{ContradictionDetectionEnabled: true}
	w := NewContradictionWorker(nil, cfg)
	// Must not panic with empty tenant
	w.OnMemoryEvent("", "root.child")
}

func TestOnMemoryEvent_EmptyPath(t *testing.T) {
	cfg := &config.Config{ContradictionDetectionEnabled: true}
	w := NewContradictionWorker(nil, cfg)
	w.OnMemoryEvent("tenant-1", "")
}

func TestOnMemoryEvent_SingleLabelPath(t *testing.T) {
	cfg := &config.Config{ContradictionDetectionEnabled: true}
	w := NewContradictionWorker(nil, cfg)
	// Single-label path has no parent prefix — must not panic.
	w.OnMemoryEvent("tenant-1", "root")
}
