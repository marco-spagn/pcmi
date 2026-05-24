//go:build integration

package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/model"
)

func seedMemoriesAtPaths(t *testing.T, app *fiber.App, paths []string) {
	t.Helper()
	for _, p := range paths {
		storeBody := fmt.Sprintf(`{"path":%q,"content":"seed-%s","embedding_model":"unspecified"}`, p, p)
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("store %q %d", p, resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestIntegrationHTTP_AuditListCursorPagination(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	since := time.Now().UTC().Add(-time.Second).Format(time.RFC3339)
	suffix := time.Now().Format("150405")
	paths := make([]string, 5)
	for i := range paths {
		paths[i] = fmt.Sprintf("root.http.auditpage.%s.%d", suffix, i)
	}
	seedMemoriesAtPaths(t, app, paths)

	auditQS := "limit=2&since=" + url.QueryEscape(since)
	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?"+auditQS, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page1 %d: %s", resp.StatusCode, b)
	}
	var page1 struct {
		Entries []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
		Total      int    `json:"total"`
		Limit      int    `json:"limit"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: entries=%d has_more=%v cursor=%q", len(page1.Entries), page1.HasMore, page1.NextCursor)
	}
	if page1.Total < 5 || page1.Limit != 2 {
		t.Fatalf("page1 total=%d limit=%d", page1.Total, page1.Limit)
	}

	page2URL := "/v1/audit?" + auditQS + "&cursor=" + url.QueryEscape(page1.NextCursor)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, page2URL, ""))
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
			ID int64 `json:"id"`
		} `json:"entries"`
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 2 {
		t.Fatalf("page2 entries=%d", len(page2.Entries))
	}
	if page2.Total < 5 {
		t.Fatalf("page2 total=%d", page2.Total)
	}
	if page1.Entries[0].ID == page2.Entries[0].ID {
		t.Fatalf("duplicate page boundary id=%d", page1.Entries[0].ID)
	}
}

func TestIntegrationHTTP_AuditListMalformedCursorRejected(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?limit=5&cursor=not-a-valid-cursor", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("audit malformed cursor %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_HistoryListCursorPagination(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.histpage." + time.Now().Format("150405")
	for i := 0; i < 5; i++ {
		storeBody := fmt.Sprintf(`{"path":%q,"content":"v%d","embedding_model":"unspecified"}`, path, i)
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("store version %d %d", i, resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}

	histPath := "/v1/memories/history?path=" + url.QueryEscape(path) + "&limit=2"
	resp, err := app.Test(reqAuthed(t, http.MethodGet, histPath, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page1 %d: %s", resp.StatusCode, b)
	}
	var page1 struct {
		Entries []struct {
			ID      int64 `json:"id"`
			Content string `json:"content"`
		} `json:"entries"`
		Total      int    `json:"total"`
		Limit      int    `json:"limit"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: entries=%d has_more=%v", len(page1.Entries), page1.HasMore)
	}
	if page1.Total < 5 || page1.Limit != 2 {
		t.Fatalf("page1 total=%d limit=%d (want full path version count)", page1.Total, page1.Limit)
	}

	page2Path := "/v1/memories/history?path=" + url.QueryEscape(path) + "&limit=2&cursor=" + url.QueryEscape(page1.NextCursor)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, page2Path, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page2 cursor %d: %s", resp.StatusCode, b)
	}
	var page2 struct {
		Entries []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 2 {
		t.Fatalf("page2 entries=%d", len(page2.Entries))
	}
	if page1.Entries[0].ID == page2.Entries[0].ID {
		t.Fatalf("duplicate boundary id=%d", page1.Entries[0].ID)
	}

	lastID := page1.Entries[len(page1.Entries)-1].ID
	page2After := "/v1/memories/history?path=" + url.QueryEscape(path) + "&limit=2&after_id=" + strconv.FormatInt(lastID, 10)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, page2After, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page2 after_id %d: %s", resp.StatusCode, b)
	}
	var page2AfterID struct {
		Entries []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2AfterID); err != nil {
		t.Fatal(err)
	}
	if len(page2AfterID.Entries) < 1 {
		t.Fatal("after_id page empty")
	}
	if page2AfterID.Entries[0].ID == lastID {
		t.Fatalf("after_id included anchor id=%d", lastID)
	}
}

func TestIntegrationHTTP_LinksListCursorPagination(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	suffix := time.Now().Format("150405")
	fromPath := "root.http.linkpage." + suffix
	seedMemoriesAtPaths(t, app, []string{fromPath})

	for i := 0; i < 5; i++ {
		toPath := fmt.Sprintf("%s.target.%d", fromPath, i)
		seedMemoriesAtPaths(t, app, []string{toPath})
		linkBody := fmt.Sprintf(`{"from_path":%q,"to_path":%q,"link_type":"related"}`, fromPath, toPath)
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/links", linkBody))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("link %d status %d", i, resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}

	listURL := "/v1/memories/links?from_path=" + url.QueryEscape(fromPath) + "&limit=2"
	resp, err := app.Test(reqAuthed(t, http.MethodGet, listURL, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page1 %d: %s", resp.StatusCode, b)
	}
	var page1 struct {
		Entries []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: entries=%d has_more=%v", len(page1.Entries), page1.HasMore)
	}

	page2URL := listURL + "&cursor=" + url.QueryEscape(page1.NextCursor)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, page2URL, ""))
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
			ID int64 `json:"id"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 2 {
		t.Fatalf("page2 entries=%d", len(page2.Entries))
	}
	if page1.Entries[0].ID == page2.Entries[0].ID {
		t.Fatalf("duplicate boundary id=%d", page1.Entries[0].ID)
	}
}

func TestIntegrationHTTP_AdminTenantsListCursorPagination(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	suffix := time.Now().Format("150405")
	slugs := make([]string, 3)
	for i := range slugs {
		slugs[i] = fmt.Sprintf("pag-tenant-%s-%d", suffix, i)
		createBody, _ := json.Marshal(map[string]any{"slug": slugs[i], "name": "Pagination Tenant"})
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/admin/tenants", string(createBody)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create tenant %d status %d", i, resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}

	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/admin/tenants?limit=1", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page1 %d: %s", resp.StatusCode, b)
	}
	var page1 struct {
		Tenants []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"tenants"`
		Total      int    `json:"total"`
		Limit      int    `json:"limit"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Tenants) != 1 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: tenants=%d has_more=%v", len(page1.Tenants), page1.HasMore)
	}
	if page1.Total < 3 || page1.Limit != 1 {
		t.Fatalf("page1 total=%d limit=%d", page1.Total, page1.Limit)
	}

	page2URL := "/v1/admin/tenants?limit=1&cursor=" + url.QueryEscape(page1.NextCursor)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, page2URL, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page2 %d: %s", resp.StatusCode, b)
	}
	var page2 struct {
		Tenants []struct {
			ID string `json:"id"`
		} `json:"tenants"`
		Total int `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Tenants) != 1 {
		t.Fatalf("page2 tenants=%d", len(page2.Tenants))
	}
	if page2.Total != page1.Total {
		t.Fatalf("total changed: %d vs %d", page1.Total, page2.Total)
	}
	if page1.Tenants[0].ID == page2.Tenants[0].ID {
		t.Fatalf("duplicate tenant boundary id=%s", page1.Tenants[0].ID)
	}
}

func TestIntegrationHTTP_DistilledListCursorPagination(t *testing.T) {
	app, pool, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	ctx := context.Background()
	tenantID, _, err := grpcserver.ResolveTenantForTest(ctx, pool, httpE2EAPIKey)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT set_tenant_context($1::uuid)`, tenantID); err != nil {
		t.Fatalf("set_tenant_context: %v", err)
	}

	prefix := "root.http.distpage." + time.Now().Format("150405")
	for i := 0; i < 5; i++ {
		distPath := fmt.Sprintf("%s.row%d", prefix, i)
		_, err := pool.Exec(ctx, `
			INSERT INTO distilled_knowledge (tenant_id, path, summary, insights, confidence_score, source_entry_ids, version)
			VALUES ($1::uuid, $2::ltree, $3, '[]'::jsonb, 0.5, '{}'::bigint[], 1)`,
			tenantID, distPath, "distilled-"+strconv.Itoa(i),
		)
		if err != nil {
			t.Fatalf("insert distilled %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	listURL := "/v1/distilled?path_prefix=" + url.QueryEscape(prefix) + "&limit=2"
	resp, err := app.Test(reqAuthed(t, http.MethodGet, listURL, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page1 %d: %s", resp.StatusCode, b)
	}
	var page1 struct {
		Entries []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
		Total      int    `json:"total"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: entries=%d has_more=%v", len(page1.Entries), page1.HasMore)
	}
	if page1.Total < 5 {
		t.Fatalf("page1 total=%d want seeded row count", page1.Total)
	}

	page2URL := listURL + "&cursor=" + url.QueryEscape(page1.NextCursor)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, page2URL, ""))
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
			ID int64 `json:"id"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 2 {
		t.Fatalf("page2 entries=%d", len(page2.Entries))
	}
	if page1.Entries[0].ID == page2.Entries[0].ID {
		t.Fatalf("duplicate boundary id=%d", page1.Entries[0].ID)
	}
}

func TestIntegrationHTTP_WebhooksListCursorPagination(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	suffix := time.Now().Format("150405")
	for i := 0; i < 5; i++ {
		whBody := fmt.Sprintf(`{"url":"https://example.invalid/hook-%s-%d","event_types":["memory.stored"]}`, suffix, i)
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/webhooks", whBody))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			t.Fatalf("register webhook %d status %d", i, resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}

	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/webhooks?limit=2", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("page1 %d: %s", resp.StatusCode, b)
	}
	var page1 struct {
		Entries []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"entries"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1: entries=%d has_more=%v", len(page1.Entries), page1.HasMore)
	}

	page2URL := "/v1/webhooks?limit=2&cursor=" + url.QueryEscape(page1.NextCursor)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, page2URL, ""))
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
			ID string `json:"id"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 2 {
		t.Fatalf("page2 entries=%d", len(page2.Entries))
	}
	if page1.Entries[0].ID == page2.Entries[0].ID {
		t.Fatalf("duplicate boundary id=%s", page1.Entries[0].ID)
	}
}

func encodePaginationCursor(t *testing.T, sortKey string, lastID int64, ts time.Time) string {
	t.Helper()
	enc, err := model.EncodeCursor(model.Cursor{
		SortKey:       sortKey,
		LastID:        lastID,
		LastTimestamp: ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func TestIntegrationHTTP_PaginationLimitClampingAudit(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	for _, tc := range []struct {
		limit    string
		wantLim  int
	}{
		{"0", 1},
		{"500", 200},
	} {
		t.Run("limit="+tc.limit, func(t *testing.T) {
			resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?limit="+tc.limit, ""))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status %d: %s", resp.StatusCode, b)
			}
			var body struct {
				Limit int `json:"limit"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Limit != tc.wantLim {
				t.Fatalf("limit=%s response limit=%d want %d", tc.limit, body.Limit, tc.wantLim)
			}
		})
	}
}

func TestIntegrationHTTP_PaginationEmptyAuditHasNoCursor(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	since := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?limit=10&since="+url.QueryEscape(since), ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Entries    []any  `json:"entries"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 0 || body.HasMore || body.NextCursor != "" {
		t.Fatalf("empty page: entries=%d has_more=%v cursor=%q", len(body.Entries), body.HasMore, body.NextCursor)
	}
}

type auditEntryRef struct {
	ID     int64  `json:"id"`
	Path   string `json:"path"`
	Method string `json:"method"`
}

func auditMemoryStoreID(e auditEntryRef) (int64, bool) {
	if e.Method != http.MethodPost || e.Path != "/v1/memories" {
		return 0, false
	}
	return e.ID, true
}

func TestIntegrationHTTP_PaginationAuditStableOrderingNoDupGap(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	suffix := time.Now().Format("150405")
	const n = 5
	paths := make([]string, n)
	for i := range paths {
		paths[i] = fmt.Sprintf("root.http.auditstable.%s.%d", suffix, i)
	}
	since := time.Now().UTC().Format(time.RFC3339)
	seedMemoriesAtPaths(t, app, paths)

	qs := "limit=2&since=" + url.QueryEscape(since)
	seen := make(map[int64]struct{})
	var pages int
	for cursor := ""; pages < 10; pages++ {
		u := "/v1/audit?" + qs
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}
		resp, err := app.Test(reqAuthed(t, http.MethodGet, u, ""))
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Entries []struct {
				ID     int64  `json:"id"`
				Path   string `json:"path"`
				Method string `json:"method"`
			} `json:"entries"`
			NextCursor string `json:"next_cursor"`
			HasMore    bool   `json:"has_more"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		for _, e := range body.Entries {
			id, ok := auditMemoryStoreID(e)
			if !ok {
				continue
			}
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate audit id %d on page %d", id, pages+1)
			}
			seen[id] = struct{}{}
		}
		if len(seen) >= n {
			break
		}
		if !body.HasMore {
			if body.NextCursor != "" {
				t.Fatal("last page must not set next_cursor when has_more is false")
			}
			break
		}
		if body.NextCursor == "" {
			t.Fatal("has_more without next_cursor")
		}
		cursor = body.NextCursor
	}
	if len(seen) < n {
		t.Fatalf("collected %d store audit ids, want %d seeded stores", len(seen), n)
	}
}

func TestIntegrationHTTP_PaginationAuditLastPagePartial(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	suffix := time.Now().Format("150405")
	since := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 5; i++ {
		p := fmt.Sprintf("root.http.auditlast.%s.%d", suffix, i)
		seedMemoriesAtPaths(t, app, []string{p})
	}

	var cursor string
	var lastPageEntries int
	for page := 0; page < 20; page++ {
		u := "/v1/audit?limit=2&since=" + url.QueryEscape(since)
		if cursor != "" {
			u += "&cursor=" + url.QueryEscape(cursor)
		}
		resp, err := app.Test(reqAuthed(t, http.MethodGet, u, ""))
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Entries []struct {
				ID int64 `json:"id"`
			} `json:"entries"`
			NextCursor string `json:"next_cursor"`
			HasMore    bool   `json:"has_more"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		lastPageEntries = len(body.Entries)
		if !body.HasMore {
			if body.NextCursor != "" {
				t.Fatal("last page must not include next_cursor when has_more is false")
			}
			break
		}
		if body.NextCursor == "" {
			t.Fatal("has_more without next_cursor")
		}
		cursor = body.NextCursor
	}
	if lastPageEntries < 1 || lastPageEntries > 2 {
		t.Fatalf("final page size=%d want 1..2", lastPageEntries)
	}
}

func TestIntegrationHTTP_PaginationAuditWrongSortKeyCursorRejected(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	bad := encodePaginationCursor(t, model.SortKeyIDDesc, 1, time.Time{})
	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?limit=5&cursor="+url.QueryEscape(bad), ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("wrong sort cursor %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_PaginationCursorAndAfterIDRejected(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.bothparams." + time.Now().Format("150405")
	resp, err := app.Test(reqAuthed(t, http.MethodGet,
		"/v1/memories/history?path="+url.QueryEscape(path)+"&cursor=abc&after_id=1", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("cursor+after_id %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_PaginationAuditAfterIDRejected(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?limit=5&after_id=1", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("audit after_id %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_PaginationHistoryAfterIDOnlyNoCursor(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.afteronly." + time.Now().Format("150405")
	for i := 0; i < 4; i++ {
		storeBody := fmt.Sprintf(`{"path":%q,"content":"v%d","embedding_model":"unspecified"}`, path, i)
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		time.Sleep(5 * time.Millisecond)
	}

	page1Path := "/v1/memories/history?path=" + url.QueryEscape(path) + "&limit=2"
	resp, err := app.Test(reqAuthed(t, http.MethodGet, page1Path, ""))
	if err != nil {
		t.Fatal(err)
	}
	var first struct {
		Entries []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(first.Entries) != 2 {
		t.Fatalf("page1 entries=%d", len(first.Entries))
	}
	anchor := first.Entries[len(first.Entries)-1].ID

	afterURL := page1Path + "&after_id=" + strconv.FormatInt(anchor, 10)
	resp, err = app.Test(reqAuthed(t, http.MethodGet, afterURL, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("after_id only %d: %s", resp.StatusCode, b)
	}
	var second struct {
		Entries []struct {
			ID int64 `json:"id"`
		} `json:"entries"`
		Total   int  `json:"total"`
		HasMore bool `json:"has_more"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if second.Total < 4 || len(second.Entries) < 1 {
		t.Fatalf("after_id page total=%d entries=%d", second.Total, len(second.Entries))
	}
	for _, e := range second.Entries {
		if e.ID >= anchor {
			t.Fatalf("after_id page included id >= anchor: id=%d anchor=%d", e.ID, anchor)
		}
	}
}

func TestIntegrationHTTP_PaginationUnsupportedCursorVersionRejected(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	raw := `{"v":99,"k":"created_at_id_desc","id":1}`
	enc := base64.RawURLEncoding.EncodeToString([]byte(raw))
	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?limit=5&cursor="+url.QueryEscape(enc), ""))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("unsupported cursor version %d: %s", resp.StatusCode, b)
	}
}
