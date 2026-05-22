package middleware

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type mapIdempotencyCache struct {
	mu    sync.Mutex
	rows  map[string]cacheRow
}

type cacheRow struct {
	body      json.RawMessage
	expiresAt time.Time
}

func (m *mapIdempotencyCache) key(tenantID, idemKey string) string {
	return tenantID + "|" + idemKey
}

func (m *mapIdempotencyCache) Get(_ context.Context, tenantID, key string) (json.RawMessage, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[m.key(tenantID, key)]
	if !ok || time.Now().After(row.expiresAt) {
		return nil, false, nil
	}
	return row.body, true, nil
}

func (m *mapIdempotencyCache) Put(_ context.Context, tenantID, key string, response json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows == nil {
		m.rows = make(map[string]cacheRow)
	}
	m.rows[m.key(tenantID, key)] = cacheRow{body: response, expiresAt: time.Now().Add(24 * time.Hour)}
	return nil
}

func idempotencyTestApp(cache IdempotencyCache, handler fiber.Handler) *fiber.App {
	app := fiber.New()
	const tenant = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(TenantContextKey, tenant)
		return c.Next()
	})
	app.Post("/v1/memories", NewIdempotencyMiddleware(cache), handler)
	return app
}

func TestIdempotencyMiddleware_MissingKey_ProcessesNormally(t *testing.T) {
	calls := 0
	app := idempotencyTestApp(&mapIdempotencyCache{}, func(c *fiber.Ctx) error {
		calls++
		return c.JSON(fiber.Map{"id": 1, "status": "stored", "version": 1})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || calls != 1 {
		t.Fatalf("status=%d calls=%d", resp.StatusCode, calls)
	}
	if resp.Header.Get(IdempotencyReplayedHeader) != "" {
		t.Fatal("unexpected replay header")
	}
}

func TestIdempotency_FirstRequest_StoresAndReturns(t *testing.T) {
	cache := &mapIdempotencyCache{}
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"id": 99, "status": "stored", "version": 1})
	})
	key := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, key)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get(IdempotencyReplayedHeader) != "" {
		t.Fatalf("status=%d replay=%q", resp.StatusCode, resp.Header.Get(IdempotencyReplayedHeader))
	}
	if _, ok := cache.rows[cache.key("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", key)]; !ok {
		t.Fatal("expected cache entry")
	}
}

func TestIdempotency_DuplicateKey_ReturnsCachedResponse(t *testing.T) {
	cache := &mapIdempotencyCache{}
	calls := 0
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		calls++
		return c.JSON(fiber.Map{"id": calls, "status": "stored", "version": calls})
	})
	key := uuid.New().String()
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
		req.Header.Set(IdempotencyKeyHeader, key)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("iter %d status %d", i, resp.StatusCode)
		}
		if i == 1 {
			if resp.Header.Get(IdempotencyReplayedHeader) != "true" {
				t.Fatal("expected replay header on duplicate")
			}
			var got, want map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			want = map[string]any{"id": float64(1), "status": "stored", "version": float64(1)}
			if got["id"] != want["id"] || got["status"] != want["status"] || got["version"] != want["version"] {
				t.Fatalf("cached body %s", body)
			}
		}
	}
	if calls != 1 {
		t.Fatalf("handler calls=%d want 1", calls)
	}
}

func TestIdempotency_DifferentKeys_CreateSeparateVersions(t *testing.T) {
	cache := &mapIdempotencyCache{}
	calls := 0
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		calls++
		return c.JSON(fiber.Map{"id": calls, "status": "stored", "version": calls})
	})
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
		req.Header.Set(IdempotencyKeyHeader, uuid.New().String())
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
}

func TestIdempotency_DifferentTenants_SameKey_Independent(t *testing.T) {
	cache := &mapIdempotencyCache{}
	key := uuid.New().String()
	tenantA := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	tenantB := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	app := fiber.New()
	handler := func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"tenant": c.Locals(TenantContextKey).(string)})
	}
	app.Post("/v1/memories",
		func(c *fiber.Ctx) error {
			if tid := c.Get("X-Test-Tenant"); tid != "" {
				c.Locals(TenantContextKey, tid)
			}
			return c.Next()
		},
		NewIdempotencyMiddleware(cache),
		handler,
	)

	for _, tid := range []string{tenantA, tenantB} {
		req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
		req.Header.Set(IdempotencyKeyHeader, key)
		req.Header.Set("X-Test-Tenant", tid)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("tenant %s status %d", tid, resp.StatusCode)
		}
	}
	if len(cache.rows) != 2 {
		t.Fatalf("cache rows=%d want 2", len(cache.rows))
	}
}

func TestIdempotency_ExpiredKey_TreatsAsNew(t *testing.T) {
	key := uuid.New().String()
	tenant := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	cache := &mapIdempotencyCache{
		rows: map[string]cacheRow{
			tenant + "|" + key: {body: json.RawMessage(`{"id":1}`), expiresAt: time.Now().Add(-time.Hour)},
		},
	}
	calls := 0
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		calls++
		return c.JSON(fiber.Map{"id": 2, "status": "stored"})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, key)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || calls != 1 {
		t.Fatalf("status=%d calls=%d", resp.StatusCode, calls)
	}
	if resp.Header.Get(IdempotencyReplayedHeader) != "" {
		t.Fatal("expired key must not replay")
	}
}
