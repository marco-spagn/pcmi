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

func TestIntegrationHTTP_RetrieveMalformedCursorStillOK(t *testing.T) {
	// Repository does not decode cursor yet; handler must not 500 on opaque garbage.
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
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("retrieve malformed cursor %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_RetrievePaginationDTOFields(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.pagedto." + time.Now().Format("150405")
	for i := 0; i < 3; i++ {
		storeBody := `{"path":"` + path + `","content":"row` + strconv.Itoa(i) + `","embedding_model":"unspecified"}`
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("store %d", resp.StatusCode)
		}
	}

	retBody, _ := json.Marshal(map[string]any{"path_prefix": path, "limit": 2})
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/retrieve", string(retBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("retrieve %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Entries    []json.RawMessage `json:"entries"`
		Total      int               `json:"total"`
		NextCursor string            `json:"next_cursor"`
		HasMore    bool              `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Total < 1 || len(out.Entries) < 1 {
		t.Fatalf("expected entries, got total=%d entries=%d", out.Total, len(out.Entries))
	}
	// Until repository wires keyset pagination, continuation fields stay at zero values.
	if out.NextCursor != "" || out.HasMore {
		t.Logf("cursor pagination active: next_cursor=%q has_more=%v", out.NextCursor, out.HasMore)
	}
}
