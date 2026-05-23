//go:build integration

package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

func withOpenAIEnv(apiKey, baseURL string) integrationHTTPOpt {
	return func(t *testing.T) {
		t.Helper()
		t.Setenv("OPENAI_API_KEY", apiKey)
		t.Setenv("OPENAI_BASE_URL", baseURL)
	}
}

func reqWithAPIKey(method, path, body, apiKey string) *http.Request {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("X-API-Key", apiKey)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestIntegrationHTTP_AdminUIForbiddenForUserKey(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	createBody, _ := json.Marshal(map[string]any{
		"tenant_id": tid,
		"name":      "integration-user-key",
		"role":      "user",
	})
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/admin/api-keys", string(createBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create user key %d: %s", resp.StatusCode, b)
	}
	var created struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.APIKey == "" {
		t.Fatal("expected plaintext api_key in create response")
	}

	resp, err = app.Test(reqWithAPIKey(http.MethodGet, "/v1/admin/ui", "", created.APIKey))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("user key on admin ui: got %d, want 403: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_AdminUIFlowTenantsAndKeys(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	slug := "ui-flow-" + time.Now().Format("150405")
	createTenant, _ := json.Marshal(map[string]any{"slug": slug, "name": "UI Flow Tenant"})
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/admin/tenants", string(createTenant)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create tenant %d: %s", resp.StatusCode, b)
	}
	var tenant struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tenant); err != nil || tenant.ID == "" {
		t.Fatalf("tenant response: id=%q err=%v", tenant.ID, err)
	}

	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/admin/tenants?limit=50", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list tenants %d", resp.StatusCode)
	}
	var listed struct {
		Tenants []struct {
			Slug string `json:"slug"`
		} `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ten := range listed.Tenants {
		if ten.Slug == slug {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created tenant %q not in list", slug)
	}

	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/admin/api-keys?tenant_id="+tenant.ID+"&limit=10", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("list api keys %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_MemoryRoutesOpenAICompatibleBaseURL(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t, withOpenAIEnv("sk-integration-test", "https://llamacpp.internal:8080/v1"))
	defer cleanup()

	path := "root.http.compat." + time.Now().Format("150405")
	storeBody := `{"path":"` + path + `","content":"compat","embedding_model":"unspecified"}`
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("store with compatible base URL %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_MemoryRoutesAzureBaseURL(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t, withOpenAIEnv("az-integration-key", "https://my-corp.openai.azure.com"))
	defer cleanup()

	path := "root.http.azure." + time.Now().Format("150405")
	storeBody := `{"path":"` + path + `","content":"azure-route","embedding_model":"unspecified"}`
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("store with azure base URL %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_RetrieveMalformedCursorRejected(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	retBody, _ := json.Marshal(map[string]any{
		"path_prefix": "root",
		"limit":       5,
		"cursor":      "not-a-valid-cursor-token",
	})
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/retrieve", string(retBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("retrieve malformed cursor %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_RetrieveCursorPagination(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.cursorpage." + time.Now().Format("150405")
	for i := 0; i < 5; i++ {
		storeBody := `{"path":"` + path + `.row` + strconv.Itoa(i) + `","content":"row` + strconv.Itoa(i) + `","embedding_model":"unspecified"}`
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("store %d", resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}

	page1Body, _ := json.Marshal(map[string]any{"path_prefix": path, "limit": 2})
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/retrieve", string(page1Body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page1 %d: %s", resp.StatusCode, b)
	}
	var page1 struct {
		Entries    []struct {
			Content string `json:"content"`
		} `json:"entries"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: entries=%d has_more=%v cursor=%q", len(page1.Entries), page1.HasMore, page1.NextCursor)
	}

	page2Body, _ := json.Marshal(map[string]any{"path_prefix": path, "limit": 2, "cursor": page1.NextCursor})
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/retrieve", string(page2Body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page2 %d: %s", resp.StatusCode, b)
	}
	var page2 struct {
		Entries []struct {
			Content string `json:"content"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 2 {
		t.Fatalf("page2 entries=%d", len(page2.Entries))
	}
	if page1.Entries[0].Content == page2.Entries[0].Content {
		t.Fatalf("duplicate page boundary: %q", page1.Entries[0].Content)
	}
}

func TestIntegrationHTTP_RetrieveCursorWithQueryRejected(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	retBody, _ := json.Marshal(map[string]any{
		"path_prefix": "root",
		"limit":       2,
		"query":       "foo",
		"cursor":      "ignored",
	})
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/retrieve", string(retBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", resp.StatusCode)
	}
}

func TestIntegrationHTTP_StoreDedupSkipSameContent(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	suffix := time.Now().Format("150405")
	path := "root.http.dedup." + suffix
	content := "dedup-integration-" + suffix
	storeBody := `{"path":"` + path + `","content":"` + content + `","embedding_model":"unspecified"}`

	req := reqAuthed(t, http.MethodPost, "/v1/memories", storeBody)
	req.Header.Set("X-Dedup-Mode", "skip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("first store %d: %s", resp.StatusCode, b)
	}
	var first struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil || first.ID == 0 {
		t.Fatalf("first id=%d err=%v", first.ID, err)
	}

	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("second store %d: %s", resp.StatusCode, b)
	}
	var second struct {
		ID         int64  `json:"id"`
		Status     string `json:"status"`
		DedupAction string `json:"dedup_action"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if second.DedupAction != model.StoreActionSkipped || second.Status != "deduplicated" || second.ID != first.ID {
		t.Fatalf("second response: id=%d status=%q action=%q", second.ID, second.Status, second.DedupAction)
	}
}

func TestIntegrationHTTP_StoreDedupLinkDifferentPath(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	suffix := time.Now().Format("150405")
	canonical := "root.http.dedup.canon." + suffix
	alias := "root.http.dedup.alias." + suffix
	content := "dedup-link-" + suffix

	storeBody := `{"path":"` + canonical + `","content":"` + content + `","embedding_model":"unspecified"}`
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("canonical store %d", resp.StatusCode)
	}

	linkBody := `{"path":"` + alias + `","content":"` + content + `","embedding_model":"unspecified"}`
	req := reqAuthed(t, http.MethodPost, "/v1/memories", linkBody)
	req.Header.Set("X-Dedup-Mode", "link")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("link store %d: %s", resp.StatusCode, b)
	}
	var out struct {
		DedupAction string `json:"dedup_action"`
		LinkedFrom  string `json:"linked_from"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.DedupAction != model.StoreActionLinked || out.LinkedFrom != alias {
		t.Fatalf("link response: action=%q linked_from=%q", out.DedupAction, out.LinkedFrom)
	}
}
