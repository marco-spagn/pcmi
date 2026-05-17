package repository

import (
	"strings"
	"testing"
)

func TestTemporalClauseContainsPlaceholder(t *testing.T) {
	clause := temporalClause("$5")
	if !strings.Contains(clause, "$5") {
		t.Fatal("expected $5 placeholder in temporal clause")
	}
}

func TestTemporalClauseHasBothBranches(t *testing.T) {
	clause := temporalClause("$3")
	if !strings.Contains(clause, "IS NULL") {
		t.Fatal("expected IS NULL branch")
	}
	if !strings.Contains(clause, "IS NOT NULL") {
		t.Fatal("expected IS NOT NULL branch")
	}
}

func TestTemporalClauseCastApplied(t *testing.T) {
	clause := temporalClause("$2")
	if !strings.Contains(clause, "$2::timestamptz") {
		t.Fatal("expected timestamptz cast in temporal clause")
	}
}

func TestScopeFiltersContainsParams(t *testing.T) {
	f := scopeFilters("3", "4")
	if !strings.Contains(f, "$3") || !strings.Contains(f, "$4") {
		t.Fatalf("expected $3 and $4 in scope filters, got: %s", f)
	}
}

func TestScopeFiltersEmptyCondition(t *testing.T) {
	f := scopeFilters("1", "2")
	// Empty string condition = no filter
	if !strings.Contains(f, "= ''") {
		t.Fatal("expected empty-string short-circuit in scope filters")
	}
}

func TestTagFiltersContainsParams(t *testing.T) {
	f := tagFilters("5", "6")
	if !strings.Contains(f, "$5") || !strings.Contains(f, "$6") {
		t.Fatalf("expected $5 and $6 in tag filters, got: %s", f)
	}
}

func TestTagFiltersHasAnyAndAllBranches(t *testing.T) {
	f := tagFilters("7", "8")
	if !strings.Contains(f, "&&") {
		t.Fatal("expected overlap operator (&&) for 'any' match")
	}
	if !strings.Contains(f, "@>") {
		t.Fatal("expected containment operator (@>) for 'all' match")
	}
}

func TestTagFiltersCardinalityGuard(t *testing.T) {
	f := tagFilters("1", "2")
	if !strings.Contains(f, "cardinality") {
		t.Fatal("expected cardinality guard to skip tag filter on empty tags")
	}
}
