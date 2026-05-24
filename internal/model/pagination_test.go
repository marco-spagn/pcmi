package model

import (
	"testing"
	"time"
)

func TestPagination_FinishInt64Page_HasMore(t *testing.T) {
	items := []int64{3, 2, 1}
	trimmed, page, err := FinishInt64Page(items, 2, SortKeyIDDesc,
		func(v int64) int64 { return v },
		func(int64) time.Time { return time.Time{} },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(trimmed) != 2 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("unexpected trimmed=%v page=%+v", trimmed, page)
	}
}

func TestPagination_FinishInt64Page_LastPage(t *testing.T) {
	items := []int64{2, 1}
	trimmed, page, err := FinishInt64Page(items, 5, SortKeyIDDesc,
		func(v int64) int64 { return v },
		func(int64) time.Time { return time.Time{} },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(trimmed) != 2 || page.HasMore || page.NextCursor != "" {
		t.Fatalf("unexpected trimmed=%v page=%+v", trimmed, page)
	}
}

func TestPagination_CursorFromAfterID(t *testing.T) {
	cur, err := CursorFromAfterID(SortKeyIDDesc, 42)
	if err != nil {
		t.Fatal(err)
	}
	if cur.LastID != 42 || cur.SortKey != SortKeyIDDesc {
		t.Fatalf("unexpected cursor %+v", cur)
	}
}
