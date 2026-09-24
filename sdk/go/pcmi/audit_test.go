package pcmi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_VerifyAudit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/audit/verify" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "checked": 3})
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, "k")
	out, err := c.VerifyAudit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out["valid"] != true || out["checked"].(float64) != 3 {
		t.Fatalf("unexpected: %v", out)
	}
}

func TestClient_ExportAudit(t *testing.T) {
	const body = "{\"type\":\"entry\"}\n{\"type\":\"trailer\"}\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audit/export" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("from_seq") != "5" || q.Get("to_seq") != "9" || q.Get("limit") != "2" {
			t.Errorf("query = %v", q)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, "k")
	got, err := c.ExportAudit(context.Background(), 5, 9, 2)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("body = %q", got)
	}
}

func TestClient_ExportAudit_defaultsAndForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"admin role required"}`))
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, "k")
	_, err := c.ExportAudit(context.Background(), 0, 0, 0)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "admin role required" {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_RetentionAndErase(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case r.Method == "PUT":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["path_prefix"] != "finance" || body["superseded_retention_days"].(float64) != 2555 || body["max_age_days"] != nil {
				t.Errorf("put body = %v", body)
			}
		case r.Method == "POST":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["dry_run"] != true || body["path_prefix"] != "users.alice" {
				t.Errorf("erase body = %v", body)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()
	c, _ := NewClient(srv.URL, "k")
	ctx := context.Background()
	days := 2555
	if _, err := c.PutRetentionPolicy(ctx, RetentionPolicy{PathPrefix: "finance", SupersededRetentionDays: &days}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListRetentionPolicies(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteRetentionPolicy(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EraseMemories(ctx, "users.alice", true, "dsr"); err != nil {
		t.Fatal(err)
	}
	want := []string{"PUT /v1/retention-policies?", "GET /v1/retention-policies?", "DELETE /v1/retention-policies?path_prefix=", "POST /v1/memories/erase?"}
	for i, w := range want {
		if i >= len(seen) || seen[i] != w {
			t.Fatalf("requests = %v, want %v", seen, want)
		}
	}
}
