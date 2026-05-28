package main

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	t.Parallel()

	if got := formatTime(time.Time{}); got != "—" {
		t.Fatalf("zero time: got %q, want —", got)
	}

	ts := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	got := formatTime(ts)
	if got == "—" || got == "" {
		t.Fatalf("non-zero time: got %q", got)
	}
}

func TestFormatOptionalTime(t *testing.T) {
	t.Parallel()

	if got := formatOptionalTime(nil); got != "—" {
		t.Fatalf("nil: got %q, want —", got)
	}

	zero := time.Time{}
	if got := formatOptionalTime(&zero); got != "—" {
		t.Fatalf("zero: got %q, want —", got)
	}

	ts := time.Date(2026, 6, 1, 8, 30, 0, 0, time.UTC)
	got := formatOptionalTime(&ts)
	if got == "—" || got == "" {
		t.Fatalf("non-zero: got %q", got)
	}
}
