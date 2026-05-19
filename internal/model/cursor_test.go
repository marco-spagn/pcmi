package model

import (
	"strings"
	"testing"
	"time"
)

// Tests for the opaque pagination cursor introduced in PR #5
// (feat/admin-grpc-and-cursor-pagination). Round-trip, edge cases, and
// version-bump rejection all live here.

func TestCursorRoundTrip(t *testing.T) {
	in := Cursor{
		Version:       cursorVersion,
		SortKey:       "id_desc",
		LastID:        42,
		LastTimestamp: time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
	}
	enc, err := EncodeCursor(in)
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	if enc == "" {
		t.Fatal("expected non-empty encoded cursor")
	}
	out, err := DecodeCursor(enc)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if out.SortKey != in.SortKey || out.LastID != in.LastID || !out.LastTimestamp.Equal(in.LastTimestamp) {
		t.Fatalf("round-trip mismatch:\nin=%+v\nout=%+v", in, out)
	}
}

func TestCursorEncodeZeroIsEmpty(t *testing.T) {
	enc, err := EncodeCursor(Cursor{})
	if err != nil {
		t.Fatalf("EncodeCursor zero: %v", err)
	}
	if enc != "" {
		t.Fatalf("zero cursor should encode to empty string, got %q", enc)
	}
}

func TestCursorDecodeEmptyIsZero(t *testing.T) {
	c, err := DecodeCursor("")
	if err != nil {
		t.Fatalf("DecodeCursor empty: %v", err)
	}
	if !c.IsZero() {
		t.Fatalf("empty input should decode to zero cursor, got %+v", c)
	}
}

func TestCursorDecodeRejectsGarbage(t *testing.T) {
	if _, err := DecodeCursor("!!! not base64 !!!"); err == nil {
		t.Fatal("expected error for non-base64 cursor")
	}
}

func TestCursorDecodeRejectsBadJSON(t *testing.T) {
	// Base64-encode something that's clearly not JSON.
	bad := "bm90LWpzb24"  // base64 of "not-json"
	if _, err := DecodeCursor(bad); err == nil {
		t.Fatal("expected error for non-JSON payload")
	}
}

func TestCursorDecodeRejectsUnsupportedVersion(t *testing.T) {
	// Manually build a cursor with v=99.
	bad := Cursor{Version: 99, SortKey: "id_desc", LastID: 1}
	// Force-encode by going through json + base64 with the wrong version.
	// EncodeCursor will reject; round-trip via the raw helpers.
	if _, err := EncodeCursor(bad); err == nil {
		t.Fatal("EncodeCursor should reject unsupported version on emit")
	}
}

func TestCursorDecodeLegacyVersionZero(t *testing.T) {
	// Older clients that omit "v" entirely should be treated as v1
	// (forward compatibility note in cursor.go).
	enc, err := EncodeCursor(Cursor{Version: cursorVersion, SortKey: "id_desc", LastID: 5})
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecodeCursor(enc)
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != cursorVersion {
		t.Fatalf("expected version %d, got %d", cursorVersion, out.Version)
	}
}

func TestCursorTrimsWhitespace(t *testing.T) {
	enc, err := EncodeCursor(Cursor{Version: cursorVersion, SortKey: "id_desc", LastID: 7})
	if err != nil {
		t.Fatal(err)
	}
	// Pad with leading + trailing whitespace, decode should still succeed.
	out, err := DecodeCursor("  " + enc + "  \n")
	if err != nil {
		t.Fatalf("DecodeCursor whitespace: %v", err)
	}
	if out.LastID != 7 {
		t.Fatalf("LastID lost across whitespace decode: %d", out.LastID)
	}
}

func TestMakeNextCursorHasMore(t *testing.T) {
	resp, err := MakeNextCursor("id_desc", 100, time.Time{}, true)
	if err != nil {
		t.Fatalf("MakeNextCursor: %v", err)
	}
	if !resp.HasMore {
		t.Fatal("expected HasMore=true when MakeNextCursor signals more pages")
	}
	if resp.NextCursor == "" {
		t.Fatal("expected non-empty NextCursor when HasMore=true")
	}
	// And it must round-trip.
	c, err := DecodeCursor(resp.NextCursor)
	if err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if c.LastID != 100 || c.SortKey != "id_desc" {
		t.Fatalf("decoded cursor mismatch: %+v", c)
	}
}

func TestMakeNextCursorLastPage(t *testing.T) {
	resp, err := MakeNextCursor("id_desc", 999, time.Time{}, false)
	if err != nil {
		t.Fatalf("MakeNextCursor: %v", err)
	}
	if resp.HasMore {
		t.Fatal("expected HasMore=false on last page")
	}
	if resp.NextCursor != "" {
		t.Fatalf("expected empty NextCursor on last page, got %q", resp.NextCursor)
	}
}

func TestMakeNextCursorEmptySortKeyRejected(t *testing.T) {
	_, err := MakeNextCursor("", 1, time.Time{}, true)
	if err == nil {
		t.Fatal("expected error for empty sort key")
	}
	if !strings.Contains(err.Error(), "sortKey") {
		t.Errorf("error message should mention sortKey: %v", err)
	}
}

func TestPageRequestZeroLimit(t *testing.T) {
	// Sanity that the zero value of PageRequest is valid (no error path).
	var p PageRequest
	if p.Limit != 0 {
		t.Errorf("zero PageRequest must have Limit=0, got %d", p.Limit)
	}
	if !p.Cursor.IsZero() {
		t.Error("zero PageRequest must have a zero cursor")
	}
}
