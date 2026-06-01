package repository

import (
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestKeysetTimeIDClause_zero(t *testing.T) {
	clause, args, err := KeysetTimeIDClause(model.Cursor{}, "time_desc", "created_at", 3)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if clause != "" {
		t.Errorf("expected empty clause for zero cursor, got: %s", clause)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}
}

func TestKeysetTimeIDClause_sortMismatch(t *testing.T) {
	cur := model.Cursor{SortKey: "id_desc", LastID: 5}
	_, _, err := KeysetTimeIDClause(cur, "time_desc", "created_at", 3)
	if err == nil {
		t.Error("expected error for sort key mismatch")
	}
}

func TestKeysetTimeIDClause_invalidID(t *testing.T) {
	cur := model.Cursor{SortKey: "time_desc", LastID: 0}
	_, _, err := KeysetTimeIDClause(cur, "time_desc", "created_at", 3)
	if err == nil {
		t.Error("expected error for invalid cursor id")
	}
}

func TestKeysetTimeIDClause_noTimestamp(t *testing.T) {
	cur := model.Cursor{SortKey: "time_desc", LastID: 5}
	clause, args, err := KeysetTimeIDClause(cur, "time_desc", "created_at", 3)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if clause == "" {
		t.Error("expected non-empty clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestKeysetTimeIDClause_withTimestamp(t *testing.T) {
	ts := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	cur := model.Cursor{SortKey: "time_desc", LastID: 5, LastTimestamp: ts}
	clause, args, err := KeysetTimeIDClause(cur, "time_desc", "created_at", 3)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if clause == "" {
		t.Error("expected non-empty clause")
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
}

func TestKeysetIDClause_zero(t *testing.T) {
	clause, args, err := KeysetIDClause(model.Cursor{}, "id_desc", 3)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if clause != "" {
		t.Errorf("expected empty clause, got: %s", clause)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}
}

func TestKeysetIDClause_sortMismatch(t *testing.T) {
	cur := model.Cursor{SortKey: "other", LastID: 5}
	_, _, err := KeysetIDClause(cur, "id_desc", 3)
	if err == nil {
		t.Error("expected error for sort key mismatch")
	}
}

func TestKeysetIDClause_invalidID(t *testing.T) {
	cur := model.Cursor{SortKey: "id_desc", LastID: 0}
	_, _, err := KeysetIDClause(cur, "id_desc", 3)
	if err == nil {
		t.Error("expected error for invalid cursor id")
	}
}

func TestKeysetIDClause_valid(t *testing.T) {
	cur := model.Cursor{SortKey: "id_desc", LastID: 10}
	clause, args, err := KeysetIDClause(cur, "id_desc", 5)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if clause == "" {
		t.Error("expected non-empty clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestKeysetCreatedAtIDClause(t *testing.T) {
	cur := model.Cursor{SortKey: "created_at_desc", LastID: 10}
	clause, args, err := KeysetCreatedAtIDClause(cur, "created_at_desc", 4)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if clause == "" {
		t.Error("expected non-empty clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestKeysetTimeClause_valid(t *testing.T) {
	ts := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	cur := model.Cursor{SortKey: "time_desc", LastTimestamp: ts}
	clause, args, err := KeysetTimeClause(cur, "time_desc", "created_at", 3)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if clause == "" {
		t.Error("expected non-empty clause")
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestKeysetTimeClause_noTimestamp(t *testing.T) {
	cur := model.Cursor{SortKey: "time_desc"}
	_, _, err := KeysetTimeClause(cur, "time_desc", "created_at", 3)
	if err == nil {
		t.Error("expected error for missing timestamp")
	}
}

func TestKeysetTimeClause_sortMismatch(t *testing.T) {
	ts := time.Now()
	cur := model.Cursor{SortKey: "wrong", LastTimestamp: ts}
	_, _, err := KeysetTimeClause(cur, "time_desc", "created_at", 3)
	if err == nil {
		t.Error("expected error for sort key mismatch")
	}
}

func TestFetchLimit(t *testing.T) {
	tests := []struct{ input, want int }{
		{0, 2},
		{-5, 2},
		{1, 2},
		{10, 11},
		{100, 101},
	}
	for _, tt := range tests {
		got := FetchLimit(tt.input)
		if got != tt.want {
			t.Errorf("FetchLimit(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
