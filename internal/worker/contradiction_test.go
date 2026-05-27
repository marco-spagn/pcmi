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
