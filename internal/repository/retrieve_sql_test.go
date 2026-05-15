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
