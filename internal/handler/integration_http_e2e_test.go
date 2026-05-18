//go:build integration

package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/event"
	grpcserver "github.com/marco-spagn/pcmi/internal/grpc"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

const httpE2EAPIKey = "testkey123"

// sseBuffer wraps bytes.Buffer so io.Copy and test assertions can run concurrently
// without tripping the race detector or observing torn reads.
type sseBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *sseBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *sseBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// drainIntegrationHTTPSSECopy tears down the client side of the SSE connection so
// io.Copy on the response body cannot block until the go test package timeout, then
// waits (with a cap) for the copy goroutine to finish.
func drainIntegrationHTTPSSECopy(t *testing.T, srv *httptest.Server, copyDone <-chan error) {
	t.Helper()
	if srv != nil {
		srv.CloseClientConnections()
	}
	select {
	case err := <-copyDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			low := strings.ToLower(err.Error())
			if !strings.Contains(low, "cancel") && !strings.Contains(low, "closed") &&
				!strings.Contains(low, "eof") && !strings.Contains(low, "broken pipe") &&
				!strings.Contains(low, "connection reset") && !strings.Contains(low, "use of closed network connection") {
				t.Logf("sse copy: %v", err)
			}
		}
	case <-time.After(20 * time.Second):
		t.Fatal("timeout waiting for SSE reader goroutine after CloseClientConnections")
	}
}

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
	cfg := config.Load()
	app.Use(middleware.RateLimitMiddleware(cfg))
	app.Use(middleware.NewAuditMiddleware(pool).Middleware())

	RegisterReadyRoutes(app, pool)
	SetupMemoryRoutes(app, pool, pool, cfg)
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

func TestIntegrationHTTP_MissingAndInvalidAPIKey(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("missing key: want 401, got %d", resp.StatusCode)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	req.Header.Set("X-API-Key", "not-a-valid-pcmi-key-xxxxxxxx")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("invalid key: want 401, got %d", resp.StatusCode)
	}
}

func TestIntegrationHTTP_ReadonlyKey(t *testing.T) {
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

	roPlain := "ro-http-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	sum := sha256.Sum256([]byte(roPlain))
	_, err = pool.Exec(ctx, `
		INSERT INTO api_keys (tenant_id, key_hash, name, role, is_active)
		VALUES ($1::uuid, $2, $3, 'readonly', true)`,
		tenantID, hex.EncodeToString(sum[:]), "e2e-readonly",
	)
	if err != nil {
		t.Fatalf("insert readonly key: %v", err)
	}

	storeReq := httptest.NewRequest(http.MethodPost, "/v1/memories", strings.NewReader(
		`{"path":"root.http.ro.denied","content":"x","embedding_model":"unspecified"}`,
	))
	storeReq.Header.Set("X-API-Key", roPlain)
	storeReq.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(storeReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("readonly store: want 403, got %d", resp.StatusCode)
	}

	statReq := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	statReq.Header.Set("X-API-Key", roPlain)
	resp, err = app.Test(statReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("readonly stats: want 200, got %d", resp.StatusCode)
	}
}

func TestIntegrationHTTP_MemoryImportAndEmbeddingsMigrate(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.import." + time.Now().Format("150405")

	impBody := `{"mode":"skip","entries":[{"path":"` + path + `","content":"imported-line","embedding_model":"unspecified","embedding_space":"default"}]}`
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories/import", impBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("import %d: %s", resp.StatusCode, b)
	}
	var impOut struct {
		Imported int `json:"imported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&impOut); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if impOut.Imported < 1 {
		t.Fatalf("imported: %+v", impOut)
	}

	migBody := `{"path_prefix":"` + path + `","target_model":"text-embedding-3-small","embedding_space":"default"}`
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/embeddings/migrate", migBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("embed migrate %d: %s", resp.StatusCode, b)
	}
}

func TestIntegrationHTTP_GetMemoryByVersionAndNotFound(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	path := "root.http.versions." + time.Now().Format("150405")
	resp, err := app.Test(reqAuthed(t, http.MethodPost, "/v1/memories",
		`{"path":"`+path+`","content":"ver1","embedding_model":"unspecified"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("store v1 %d", resp.StatusCode)
	}
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/memories",
		`{"path":"`+path+`","content":"ver2","embedding_model":"unspecified"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("store v2 %d", resp.StatusCode)
	}

	u := "/v1/memories/" + url.PathEscape(path) + "?version=1"
	req := httptest.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("X-API-Key", httpE2EAPIKey)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("get v1 %d: %s", resp.StatusCode, b)
	}
	var ent struct {
		Content string `json:"content"`
		Version int    `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ent); err != nil {
		t.Fatal(err)
	}
	if ent.Content != "ver1" || ent.Version != 1 {
		t.Fatalf("entry: %+v", ent)
	}

	missing := "/v1/memories/root.http.absent." + time.Now().Format("150405")
	req = httptest.NewRequest(http.MethodGet, missing, nil)
	req.Header.Set("X-API-Key", httpE2EAPIKey)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("missing memory: want 404, got %d", resp.StatusCode)
	}
}

func TestIntegrationHTTP_AdminAPIKeysCreateAndRotate(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	tid := "00000000-0000-0000-0000-000000000000"
	resp, err := app.Test(reqAuthed(t, http.MethodGet, "/v1/admin/api-keys?tenant_id="+tid+"&limit=20", ""))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("list keys %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	createBody, _ := json.Marshal(map[string]any{
		"tenant_id": tid,
		"name":      "e2e-integration-key",
		"role":      "user",
	})
	resp, err = app.Test(reqAuthed(t, http.MethodPost, "/v1/admin/api-keys", string(createBody)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create key %d: %s", resp.StatusCode, b)
	}
	var created struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.APIKey == "" {
		t.Fatalf("create response: %+v", created)
	}

	rotURL := "/v1/admin/api-keys/" + url.PathEscape(created.ID) + "/rotate"
	resp, err = app.Test(reqAuthed(t, http.MethodPost, rotURL, `{"name":"e2e-rotated"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("rotate %d: %s", resp.StatusCode, b)
	}
	var rotated struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rotated); err != nil {
		t.Fatalf("decode rotate: %v", err)
	}
	if rotated.APIKey == "" || rotated.APIKey == created.APIKey {
		t.Fatalf("expected new api_key after rotate")
	}
}

func TestIntegrationHTTP_ValidationErrors(t *testing.T) {
	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	cases := []struct {
		name string
		req  *http.Request
		want int
	}{
		{
			"rollback missing version",
			reqAuthed(t, http.MethodPost, "/v1/memories/rollback", `{"path":"root.x"}`),
			400,
		},
		{
			"compact missing path",
			reqAuthed(t, http.MethodPost, "/v1/memories/compact", `{}`),
			400,
		},
		{
			"distilled missing prefix",
			reqAuthed(t, http.MethodGet, "/v1/distilled", ""),
			400,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := app.Test(tc.req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("got status %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestIntegrationHTTP_EventStreamMemoryStored(t *testing.T) {
	// gofiber's adaptor.FiberApp copies streamed bodies through fasthttp in a way that
	// can wedge httptest clients under -race on GitHub runners; integration-smoke and
	// E2E jobs exercise the same /v1/events + memory.stored path on a listening server.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("SSE via adaptor+httptest is unreliable on GHA; covered by integration-smoke / E2E")
	}

	app, _, cleanup := newIntegrationHTTPApp(t)
	defer cleanup()

	srv := httptest.NewServer(adaptor.FiberApp(app))
	defer func() {
		srv.CloseClientConnections()
		srv.Close()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf sseBuffer
	startGate := make(chan error, 1)
	finalErr := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/v1/events?types=memory.stored", nil)
		if err != nil {
			startGate <- err
			finalErr <- err
			return
		}
		req.Header.Set("X-API-Key", httpE2EAPIKey)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			startGate <- err
			finalErr <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			e := fmt.Errorf("sse status %d", resp.StatusCode)
			startGate <- e
			finalErr <- e
			return
		}
		// Unblock the test only after headers are received: Stream() has already subscribed
		// to Redis, so POST must not run before this gate or the first publish can be lost.
		startGate <- nil

		// Long-lived SSE + Fiber's adaptor.FiberApp can leave io.Copy blocked even after
		// request context cancellation; closing the body unblocks the reader so finalErr completes.
		copyDone := make(chan error, 1)
		go func() {
			_, err := io.Copy(&buf, resp.Body)
			copyDone <- err
		}()
		var copyErr error
		select {
		case <-ctx.Done():
			_ = resp.Body.Close()
			copyErr = <-copyDone
		case copyErr = <-copyDone:
			_ = resp.Body.Close()
		}
		finalErr <- copyErr
	}()

	select {
	case err := <-startGate:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for SSE GET (need subscription live before POST)")
	}

	path := "root.http.sse." + time.Now().Format("150405")
	storeBody := `{"path":"` + path + `","content":"sse-body","embedding_model":"unspecified"}`
	postReq, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", strings.NewReader(storeBody))
	if err != nil {
		t.Fatal(err)
	}
	postReq.Header.Set("X-API-Key", httpE2EAPIKey)
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(postReq)
	if err != nil {
		t.Fatal(err)
	}
	bb, _ := io.ReadAll(postResp.Body)
	_ = postResp.Body.Close()
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("store %d: %s", postResp.StatusCode, bb)
	}

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		s := buf.String()
		if strings.Contains(s, "memory.stored") && strings.Contains(s, path) {
			cancel()
			drainIntegrationHTTPSSECopy(t, srv, finalErr)
			return
		}
		time.Sleep(40 * time.Millisecond)
	}

	cancel()
	drainIntegrationHTTPSSECopy(t, srv, finalErr)
	t.Fatalf("timeout waiting for memory.stored SSE; buf=%q", buf.String())
}
