package middleware

import (
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

func TestIdempotency_PutError_StillReturns200(t *testing.T) {
	t.Parallel()
	cache := &errIdempotencyCache{putErr: errors.New("redis down")}
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		return c.JSON(map[string]bool{"ok": true})
	})
	key := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, key)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestIdempotency_ConcurrentDifferentKeys_AllExecute(t *testing.T) {
	t.Parallel()
	cache := &mapIdempotencyCache{}
	var calls atomic.Int32
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		calls.Add(1)
		return c.JSON(map[string]int32{"n": calls.Load()})
	})

	const n = 6
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
			req.Header.Set(IdempotencyKeyHeader, uuid.New().String())
			resp, _ := app.Test(req, -1)
			if resp != nil {
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	if calls.Load() != n {
		t.Fatalf("calls=%d want %d", calls.Load(), n)
	}
}

func TestIdempotency_CachedJSON_roundTrips(t *testing.T) {
	t.Parallel()
	cache := &mapIdempotencyCache{}
	key := uuid.New().String()
	want := map[string]any{"id": "mem-1", "status": "stored"}
	app := idempotencyTestApp(cache, func(c *fiber.Ctx) error {
		return c.JSON(want)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/memories", nil)
	req.Header.Set(IdempotencyKeyHeader, key)

	resp1, _ := app.Test(req)
	body1, _ := ioReadAll(resp1)
	resp1.Body.Close()

	resp2, _ := app.Test(req)
	body2, _ := ioReadAll(resp2)
	resp2.Body.Close()

	if string(body1) != string(body2) {
		t.Fatalf("bodies differ: %s vs %s", body1, body2)
	}
	var got map[string]any
	if err := json.Unmarshal(body2, &got); err != nil || got["id"] != "mem-1" {
		t.Fatalf("cached=%v err=%v", got, err)
	}
}

func ioReadAll(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, errors.New("nil response")
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
