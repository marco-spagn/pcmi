//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/event"
	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

const httpE2EAPIKey = "testkey123"

func newIntegrationHTTPApp(t *testing.T) (*fiber.App, *pgxpool.Pool, func()) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}
	t.Setenv("RATE_LIMIT_DISABLED", "true")
	t.Setenv("OPENAI_API_KEY", "")

	pool, err := pgxpool.New(t.Context(), dbURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	mr, err := miniredis.Run()
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	event.InitRedis(mr.Addr())

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(middleware.APIKeyMiddleware(pool))
	app.Use(middleware.RateLimitMiddleware())
	app.Use(middleware.NewAuditMiddleware(pool).Middleware())

	RegisterReadyRoutes(app, pool)
	SetupMemoryRoutes(app, pool, pool)
	SetupAdminRoutes(app, pool)

	cleanup := func() {
		_ = app.Shutdown()
		mr.Close()
		pool.Close()
	}

	return app, pool, cleanup
}

func reqAuthed(t *testing.T, method, path string, body string) *http.Request {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("X-API-Key", httpE2EAPIKey)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestIntegrationHTTP_ReadyAndHealth(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	for _, p := range []string{"/ready", "/v1/ready"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s: %d %s", p, resp.StatusCode, b)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("X-API-Key", httpE2EAPIKey)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("health %d", resp.StatusCode)
	}
}

func TestIntegrationHTTP_MemoryCRUDAndRoutes(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.e2e." + time.Now().Format("150405")

	// Store
	storeBody := `{"path":"` + path + `","content":"http-e2e-body","tags":["e2e"],"embedding_model":"unspecified","embedding_space":"default"}`
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("store %d: %s", resp.StatusCode, b)
	}

	// GET by wildcard path
	getURL := "/v1/memories/" + strings.TrimPrefix(path, "")
	req := httptest.NewRequest(http.MethodGet, getURL, nil)
	req.Header.Set("X-API-Key", httpE2EAPIKey)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get %d: %s", resp.StatusCode, b)
	}

	// Retrieve
	retBody := `{"path_prefix":"` + path + `","limit":5}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/retrieve", retBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("retrieve %d: %s", resp.StatusCode, b)
	}

	// Stats
	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/stats", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("stats %d", resp.StatusCode)
	}

	// Schemas
	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/events/schemas", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("schemas %d", resp.StatusCode)
	}

	// Refine (Redis)
	refineBody := `{"path_prefix":"` + path + `"}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/refine", refineBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("refine %d: %s", resp.StatusCode, b)
	}

	// Links
	linkBody := `{"from_path":"` + path + `","to_path":"` + path + `.linked","link_type":"related"}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/links", linkBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("link post %d: %s", resp.StatusCode, b)
	}

	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/memories/links?from_path="+path, ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("link list %d", resp.StatusCode)
	}

	// History
	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/memories/history?path="+path+"&limit=10", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("history %d: %s", resp.StatusCode, b)
	}

	// Lineage memory
	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/lineage/memory?path="+path, ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("lineage %d: %s", resp.StatusCode, b)
	}

	// Summarize (extractive — no OpenAI)
	sumBody := `{"path_prefix":"` + path + `","limit":5}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/summarize", sumBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("summarize %d: %s", resp.StatusCode, b)
	}

	// Audit list
	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/audit?limit=5", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("audit %d", resp.StatusCode)
	}

	// Batch store
	batchBody := `{"items":[{"path":"` + path + `.batch1","content":"b1","embedding_model":"unspecified"}]}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/batch", batchBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("batch %d: %s", resp.StatusCode, b)
	}

	// Batch retrieve
	brBody := `{"queries":[{"path_prefix":"` + path + `","limit":3}]}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/retrieve/batch", brBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("batch retrieve %d: %s", resp.StatusCode, b)
	}

	// Export
	expBody := `{"path_prefix":"` + path + `","limit":10,"include_embeddings":false}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/export", expBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("export %d: %s", resp.StatusCode, b)
	}

	// Webhook register + list
	whBody := `{"url":"https://example.invalid/hook","event_types":["memory.stored"]}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/webhooks", whBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("webhook %d: %s", resp.StatusCode, b)
	}

	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/webhooks", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("webhook list %d", resp.StatusCode)
	}

	resp, err = app.Test(reqAuthed(t, http.MethodGet, "/v1/webhooks/dead-letter", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("dlq %d", resp.StatusCode)
	}

	// Ingest event
	ingBody := `{"event_type":"integration.http","payload":{"k":1}}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/events", ingBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("ingest %d: %s", resp.StatusCode, b)
	}

	// Compact
	compBody := `{"path":"` + path + `","keep_superseded":20}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/compact", compBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("compact %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_AdminTenants(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/admin/tenants?limit=5", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("list tenants %d: %s", resp.StatusCode, b)
	}

	slug := "http-e2e-" + time.Now().Format("150405")
	body, _ := json.Marshal(map[string]any{"slug": slug, "name": "E2E Tenant", "settings": map[string]any{}})
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/admin/tenants", string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create tenant %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_DistilledLineageAndRollback(t *testing.T) {
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

	path := "root.http.distill." + time.Now().Format("150405")

	storeVersion := func(content string) int64 {
		storeBody := `{"path":"` + path + `","content":"` + content + `","embedding_model":"unspecified"}`
		resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
		if err != nil {
			t.Fatalf("store: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("store %d: %s", resp.StatusCode, b)
		}
		var out struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode store: %v", err)
		}
		return out.ID
	}

	entryIDv1 := storeVersion("http-rollback-v1")
	_ = storeVersion("http-rollback-v2")

	getContent := func() string {
		getURL := "/v1/memories/" + path
		req := httptest.NewRequest(http.MethodGet, getURL, nil)
		req.Header.Set("X-API-Key", httpE2EAPIKey)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("get %d: %s", resp.StatusCode, b)
		}
		var ent struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ent); err != nil {
			t.Fatalf("decode entry: %v", err)
		}
		return ent.Content
	}

	if getContent() != "http-rollback-v2" {
		t.Fatalf("expected v2 content before rollback")
	}

	distPath := path + ".distilled"
	var distillID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO distilled_knowledge (tenant_id, path, summary, insights, confidence_score, source_entry_ids, version)
		VALUES ($1::uuid, $2::ltree, $3, '[]'::jsonb, 0.9, $4::bigint[], 1)
		RETURNING id`,
		tenantID, distPath, "http e2e distilled", []int64{entryIDv1},
	).Scan(&distillID)
	if err != nil {
		t.Fatalf("insert distilled: %v", err)
	}

	distReq := httptest.NewRequest(http.MethodGet, "/v1/distilled?path_prefix="+url.QueryEscape(path)+"&limit=10", nil)
	distReq.Header.Set("X-API-Key", httpE2EAPIKey)
	resp, err := app.Test(distReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("distilled list %d: %s", resp.StatusCode, b)
	}
	var distList struct {
		Entries []map[string]any `json:"entries"`
		Total   int              `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&distList); err != nil {
		t.Fatalf("decode distilled: %v", err)
	}
	if distList.Total < 1 {
		t.Fatalf("distilled list empty: %+v", distList)
	}

	linURL := "/v1/lineage/distilled/" + strconv.FormatInt(distillID, 10)
	linReq := httptest.NewRequest(http.MethodGet, linURL, nil)
	linReq.Header.Set("X-API-Key", httpE2EAPIKey)
	resp, err = app.Test(linReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("distilled lineage %d: %s", resp.StatusCode, b)
	}

	rbBody := `{"path":"` + path + `","version":1}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/rollback", rbBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("rollback %d: %s", resp.StatusCode, b)
	}

	if getContent() != "http-rollback-v1" {
		t.Fatalf("after rollback want v1 content, got %q", getContent())
	}
}

func TestIntegrationHTTP_RollbackNotFound(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.badrollback." + time.Now().Format("150405")
	storeBody := `{"path":"` + path + `","content":"only-one","embedding_model":"unspecified"}`
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories", storeBody))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("store %d", resp.StatusCode)
	}

	rbBody := `{"path":"` + path + `","version":99}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/rollback", rbBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, b)
	}
}
