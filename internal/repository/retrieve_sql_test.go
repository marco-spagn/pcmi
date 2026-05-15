package repository

import (
	"strings"
	"testing"
)

func TestTemporalClauseCurrentSlice(t *testing.T) {
	clause := temporalClause("$4")
	if !strings.Contains(clause, "valid_to IS NULL") {
		t.Fatal("expected current-slice branch")
	}
	if !strings.Contains(clause, "valid_from <=") {
		t.Fatal("expected as_of branch")
	}
}

func TestScopeFiltersOptional(t *testing.T) {
	f := scopeFilters("5", "6")
	if !strings.Contains(f, "source_agent_id") {
		t.Fatal("expected agent filter")
	}
	if !strings.Contains(f, "embedding_space") {
		t.Fatal("expected embedding space filter")
	}
}

func TestTagFiltersAnyAndAll(t *testing.T) {
	f := tagFilters("3", "4")
	if !strings.Contains(f, "tags &&") {
		t.Fatal("expected overlap (any) filter")
	}
	if !strings.Contains(f, "tags @>") {
		t.Fatal("expected containment (all) filter")
	}
}
