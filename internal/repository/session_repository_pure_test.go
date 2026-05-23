package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/marco-spagn/pcmi/internal/model"
)

func TestSessionStatus(t *testing.T) {
	t.Parallel()
	if sessionStatus(nil) != "active" {
		t.Fatal("nil ended_at should be active")
	}
	ended := time.Now()
	if sessionStatus(&ended) != "ended" {
		t.Fatal("ended_at set should be ended")
	}
}

func TestPromotePath(t *testing.T) {
	t.Parallel()
	sid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	underscore := "sessions.aaaaaaaa_aaaa_aaaa_aaaa_aaaaaaaaaaaa."
	tests := []struct {
		name   string
		path   string
		target string
		want   string
	}{
		{"underscore prefix", underscore + "note", "root", "root.note"},
		{"uuid prefix", "sessions." + sid + ".wm", "root", "root.wm"},
		{"bare sessions suffix", "sessions.misc", "root", "root.misc"},
		{"empty suffix", underscore, "root", "root"},
		{"custom target", underscore + "a.b", "archive", "archive.a.b"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := promotePath(tc.path, sid, tc.target)
			if got != tc.want {
				t.Fatalf("promotePath(%q) = %q want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestCloneMetadata(t *testing.T) {
	t.Parallel()
	src := map[string]any{"session_id": "s1", "k": 1}
	out := cloneMetadata(src)
	out["k"] = 2
	if src["k"] != 1 {
		t.Fatal("clone must not mutate source")
	}
	delete(out, "session_id")
	if _, ok := src["session_id"]; !ok {
		t.Fatal("source session_id must remain")
	}
}

func TestSessionScopedStoreRequest(t *testing.T) {
	t.Parallel()
	sid := uuid.New().String()
	req := model.SessionStoreMemoryRequest{
		Path:    "note",
		Content: "hello",
		Metadata: map[string]any{"x": 1},
		Tags:    []string{"t"},
	}
	store := SessionScopedStoreRequest(sid, req)
	if store.Content != "hello" {
		t.Fatalf("content=%q", store.Content)
	}
	if store.Metadata["session_id"] != sid {
		t.Fatalf("metadata session_id=%v", store.Metadata["session_id"])
	}
	if store.Metadata["memory_scope"] != sessionScopeWorking {
		t.Fatalf("scope=%v", store.Metadata["memory_scope"])
	}
	if store.EmbeddingModel != "unspecified" {
		t.Fatalf("embedding_model=%q", store.EmbeddingModel)
	}
	if !strings.HasPrefix(store.Path, "sessions.") {
		t.Fatalf("path=%q", store.Path)
	}
}
