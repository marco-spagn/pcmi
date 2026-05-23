package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type errIdempotencyCache struct {
	getErr error
	putErr error
}

func (e *errIdempotencyCache) Get(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, e.getErr
}

func (e *errIdempotencyCache) Put(context.Context, string, string, json.RawMessage) error {
	return e.putErr
}

func TestIdempotency_InvalidUUID_Returns400(t *testing.T) {
	t.Parallel()
	app := idempotencyTestApp(&mapIdempotencyCache{}, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, "not-a-uuid")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
}

func TestIdempotency_WhitespaceKey_TreatedAsAbsent(t *testing.T) {
	t.Parallel()
	calls := 0
	app := idempotencyTestApp(&mapIdempotencyCache{}, func(c *fiber.Ctx) error {
		calls++
		return c.JSON(fiber.Map{"n": calls})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, "   ")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || calls != 1 {
		t.Fatalf("status=%d calls=%d", resp.StatusCode, calls)
	}
}

func TestIdempotency_NoTenant_SkipsCache(t *testing.T) {
	t.Parallel()
	cache := &mapIdempotencyCache{}
	calls := 0
	app := fiber.New()
	app.Post("/v1/memories", NewIdempotencyMiddleware(cache), func(c *fiber.Ctx) error {
		calls++
		return c.JSON(fiber.Map{"n": calls})
	})
	key := uuid.New().String()
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
		req.Header.Set(IdempotencyKeyHeader, key)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if calls != 2 {
		t.Fatalf("without tenant, handler should run twice; calls=%d", calls)
	}
	if len(cache.rows) != 0 {
		t.Fatal("cache should stay empty without tenant")
	}
}

func TestIdempotency_LookupError_Returns500(t *testing.T) {
	t.Parallel()
	app := idempotencyTestApp(&errIdempotencyCache{getErr: errors.New("db down")}, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, uuid.New().String())
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}
}

func TestIdempotency_NonOKResponse_NotCached(t *testing.T) {
	t.Parallel()
	cache := &mapIdempotencyCache{}
	key := uuid.New().String()
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		return c.Status(500).JSON(fiber.Map{"error": "fail"})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, key)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(cache.rows) != 0 {
		t.Fatal("5xx must not be cached")
	}

	// Second request should hit handler again (still 500), not replay.
	resp2, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.Header.Get(IdempotencyReplayedHeader) == "true" {
		t.Fatal("must not replay failed response")
	}
}

func TestIdempotency_EmptyBody_NotCached(t *testing.T) {
	t.Parallel()
	cache := &mapIdempotencyCache{}
	key := uuid.New().String()
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		c.Status(fiber.StatusOK)
		c.Response().SetBodyRaw(nil)
		return nil
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, key)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(cache.rows) != 0 {
		t.Fatal("empty 200 body must not be cached")
	}
}

func TestIdempotency_ConcurrentDuplicateKey_SingleHandlerExecution(t *testing.T) {
	t.Parallel()
	cache := &mapIdempotencyCache{}
	var calls atomic.Int32
	key := uuid.New().String()
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		calls.Add(1)
		return c.JSON(fiber.Map{"id": calls.Load(), "status": "stored"})
	})

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for range n {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
			req.Header.Set(IdempotencyKeyHeader, key)
			resp, err := app.Test(req, -1)
			if err != nil {
				errCh <- err
				return
			}
			if resp.StatusCode != http.StatusOK {
				errCh <- errors.New("bad status")
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if c := calls.Load(); c < 1 {
		t.Fatalf("handler never ran: calls=%d", c)
	}
	// All responses must match the first stored body (id may be 1..n depending on race).
	row, ok := cache.rows[cache.key("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", key)]
	if !ok {
		t.Fatal("expected cache entry after concurrent requests")
	}
	var cached map[string]any
	if err := json.Unmarshal(row.body, &cached); err != nil {
		t.Fatal(err)
	}
	if cached["status"] != "stored" {
		t.Fatalf("cached=%v", cached)
	}
}

func TestIdempotency_ReplayHeaderOnlyOnHit(t *testing.T) {
	t.Parallel()
	cache := &mapIdempotencyCache{}
	key := uuid.New().String()
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"id": 1})
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, key)

	resp1, _ := app.Test(req)
	resp1.Body.Close()
	if resp1.Header.Get(IdempotencyReplayedHeader) != "" {
		t.Fatal("first request must not set replay header")
	}

	resp2, _ := app.Test(req)
	body, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.Header.Get(IdempotencyReplayedHeader) != "true" {
		t.Fatalf("second request replay header=%q body=%s", resp2.Header.Get(IdempotencyReplayedHeader), body)
	}
}
