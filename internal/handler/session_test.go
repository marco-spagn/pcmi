//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func applySessionsMigration(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var exists bool
	err := pool.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'agent_sessions'
		)`).Scan(&exists)
	if err != nil {
		t.Fatalf("check agent_sessions: %v", err)
	}
	if exists {
		return
	}
	sqlBytes, err := os.ReadFile("migrations/016_sessions.sql")
	if err != nil {
		// tests run from package dir; try repo root
		sqlBytes, err = os.ReadFile("../../migrations/016_sessions.sql")
	}
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(context.Background(), string(sqlBytes)); err != nil {
		t.Fatalf("apply 016_sessions: %v", err)
	}
}

func postSession(t *testing.T, app *fiber.App, body string) string {
	t.Helper()
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/sessions", body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create session %d: %s", resp.StatusCode, b)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.ID == "" {
		t.Fatalf("session id: %v", err)
	}
	return out.ID
}

func TestSession_Create_ReturnsID(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()
	applySessionsMigration(t, pool)

	id := postSession(t, app, `{"metadata":{"purpose":"test"}}`)
	if id == "" {
		t.Fatal("expected session id")
	}
}

func TestSession_StoreMemory_ScopedToSession(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()
	applySessionsMigration(t, pool)

	sid := postSession(t, app, `{}`)
	suffix := time.Now().Format("150405")
	body := fmt.Sprintf(`{"path":"note.%s","content":"working-%s"}`, suffix, suffix)
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/sessions/"+sid+"/memories", body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("store session memory %d: %s", resp.StatusCode, b)
	}

	var meta string
	err = pool.QueryRow(context.Background(), `
		SELECT metadata->>'session_id' FROM memory_entries
		WHERE metadata->>'session_id' = $1 AND valid_to IS NULL LIMIT 1`, sid).Scan(&meta)
	if err != nil || meta != sid {
		t.Fatalf("session_id in metadata: got %q err=%v", meta, err)
	}
}

func TestSession_Retrieve_SessionMemoriesFirst(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()
	applySessionsMigration(t, pool)

	sid := postSession(t, app, `{}`)
	suffix := time.Now().Format("150405")

	// Long-term memory (no session)
	ltPath := "root.session.lt." + suffix
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories",
		fmt.Sprintf(`{"path":%q,"content":"long-term","metadata":{}}`, ltPath)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Working memory in session
	smBody := fmt.Sprintf(`{"path":"wm.%s","content":"session-first"}`, suffix)
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/sessions/"+sid+"/memories", smBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	url := fmt.Sprintf("/v1/sessions/%s/memories?include_long_term=true&path_prefix=root.session.lt&limit=10", sid)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, url, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("list memories %d: %s", resp.StatusCode, b)
	}
	var listed struct {
		Entries []struct {
			Content  string `json:"content"`
			Metadata map[string]any `json:"metadata"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(listed.Entries))
	}
	if listed.Entries[0].Content != "session-first" {
		t.Fatalf("session memory should be first, got %q", listed.Entries[0].Content)
	}
}

func TestSession_Promote_CopiesToLongTerm(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()
	applySessionsMigration(t, pool)

	sid := postSession(t, app, `{}`)
	suffix := time.Now().Format("150405")
	storeBody := fmt.Sprintf(`{"path":"promote.%s","content":"promote-me"}`, suffix)
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/sessions/"+sid+"/memories", storeBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/sessions/"+sid+"/promote", `{"target_prefix":"root"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("promote %d: %s", resp.StatusCode, b)
	}
	var promoted struct {
		Promoted int `json:"promoted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&promoted); err != nil || promoted.Promoted < 1 {
		t.Fatalf("promoted count: %+v err=%v", promoted, err)
	}

	var sessionID *string
	err = pool.QueryRow(context.Background(), `
		SELECT metadata->>'session_id' FROM memory_entries
		WHERE content = 'promote-me' AND valid_to IS NULL LIMIT 1`).Scan(&sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sessionID != nil && *sessionID != "" {
		t.Fatalf("expected session_id cleared after promote, got %q", *sessionID)
	}
}

func TestSession_End_SetsEndedAt(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()
	applySessionsMigration(t, pool)

	sid := postSession(t, app, `{}`)
	req := reqAuthed(t, http.MethodDelete, "/v1/sessions/"+sid, "")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("end session %d: %s", resp.StatusCode, b)
	}
	var endedAt *time.Time
	err = pool.QueryRow(context.Background(), `
		SELECT ended_at FROM agent_sessions WHERE id = $1::uuid`, sid).Scan(&endedAt)
	if err != nil || endedAt == nil {
		t.Fatalf("ended_at not set: err=%v ended=%v", err, endedAt)
	}
}

func TestSession_MultiAgentSessions_Isolated(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()
	applySessionsMigration(t, pool)

	s1 := postSession(t, app, `{"metadata":{"agent":"one"}}`)
	s2 := postSession(t, app, `{"metadata":{"agent":"two"}}`)
	suffix := time.Now().Format("150405")

	for _, pair := range []struct{ sid, content string }{
		{s1, "agent-one-" + suffix},
		{s2, "agent-two-" + suffix},
	} {
		body := fmt.Sprintf(`{"path":"iso.%s","content":%q}`, suffix, pair.content)
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/sessions/"+pair.sid+"/memories", body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	var count int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM memory_entries
		WHERE metadata->>'session_id' = $1 AND valid_to IS NULL`, s1).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("session1 count: %d err=%v", count, err)
	}
	err = pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM memory_entries
		WHERE metadata->>'session_id' = $1 AND valid_to IS NULL`, s2).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("session2 count: %d err=%v", count, err)
	}

	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/sessions/"+s1+"/memories", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var listed struct {
		Entries []struct{ Content string `json:"content"` } `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Content != "agent-one-"+suffix {
		t.Fatalf("session1 isolation: %+v", listed.Entries)
	}
}
