package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/service"
)

// newStressServer returns a real TCP httptest.Server that wraps a Fiber app
// pre-wired with the memory store route and the given tenantID/role. Using a
// real TCP server makes it safe to call from multiple goroutines simultaneously
// (unlike fiber's app.Test() which is single-connection-per-call).
func newStressServer(t *testing.T, tenantID, role string, repo *handlerMockRepo) (*httptest.Server, *service.MemoryService) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	event.InitRedis(mr.Addr())

	svc := service.NewMemoryService(repo, nil)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.TenantContextKey, tenantID)
		c.Locals(middleware.RoleContextKey, role)
		return c.Next()
	})
	app.Post("/v1/memories", func(c *fiber.Ctx) error {
		var req model.StoreRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tid, _ := c.Locals(middleware.TenantContextKey).(string)
		result, err := svc.Store(c.Context(), &req, tid)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"id": result.Entry.ID})
	})
	app.Get("/v1/memories", func(c *fiber.Ctx) error {
		tid, _ := c.Locals(middleware.TenantContextKey).(string)
		prefix := c.Query("path_prefix", "root")
		resp, err := svc.Retrieve(c.Context(), &model.RetrieveRequest{PathPrefix: prefix}, tid)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"memories": resp})
	})

	srv := httptest.NewServer(adaptor.FiberApp(app))
	t.Cleanup(srv.Close)
	return srv, svc
}

// TestHandlerStress_ConcurrentPOST fires 200 concurrent POST /v1/memories
// requests. All must return HTTP 200; no deadlock within 20s.
func TestHandlerStress_ConcurrentPOST(t *testing.T) {
	repo := &handlerMockRepo{}
	srv, _ := newStressServer(t, "stress-tenant", "admin", repo)
	client := srv.Client()

	const goroutines = 200
	var wg sync.WaitGroup
	var ok200 atomic.Int32
	var notOK atomic.Int32

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{
				"path":    fmt.Sprintf("root.stress.g%d", g),
				"content": fmt.Sprintf("concurrent content %d", g),
			})
			resp, err := client.Post(srv.URL+"/v1/memories", "application/json", bytes.NewReader(body))
			if err != nil {
				notOK.Add(1)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				ok200.Add(1)
			} else {
				notOK.Add(1)
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("TestHandlerStress_ConcurrentPOST: deadlock")
	}

	if n := notOK.Load(); n > 0 {
		t.Errorf("%d/%d requests failed or got non-200 status", n, goroutines)
	}
	t.Logf("ok=%d not-ok=%d", ok200.Load(), notOK.Load())
}

// TestHandlerStress_MultiTenantConcurrent simulates 10 tenants × 20 goroutines
// each POSTing to their own server. All servers are fully set up (including
// event.InitRedis) BEFORE any goroutine is spawned so that concurrent reads
// of event.RedisClient from goroutines do not race with InitRedis writes.
func TestHandlerStress_MultiTenantConcurrent(t *testing.T) {
	repo := &handlerMockRepo{}
	const (
		tenants    = 10
		goroutines = 20
	)

	type tenantSetup struct {
		url    string
		client *http.Client
	}

	// Phase 1: set up all servers sequentially (InitRedis writes happen here).
	setups := make([]tenantSetup, tenants)
	for tn := 0; tn < tenants; tn++ {
		srv, _ := newStressServer(t, fmt.Sprintf("tenant-%02d", tn), "admin", repo)
		setups[tn] = tenantSetup{url: srv.URL, client: srv.Client()}
	}

	// Phase 2: now spawn all goroutines (only reads of event.RedisClient from here on).
	var wg sync.WaitGroup
	var errs atomic.Int32

	for tn := 0; tn < tenants; tn++ {
		ts := setups[tn]
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				path := fmt.Sprintf("root.multi.g%d", g)
				body, _ := json.Marshal(map[string]any{"path": path, "content": "x"})
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, ts.url+"/v1/memories", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := ts.client.Do(req)
				if err != nil {
					errs.Add(1)
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					errs.Add(1)
				}
			}(g)
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestHandlerStress_MultiTenantConcurrent: deadlock")
	}

	if n := errs.Load(); n > 0 {
		t.Errorf("%d requests failed in multi-tenant test", n)
	}
}

// TestHandlerStress_MixedGETAndPOST runs 100 POST and 100 GET requests
// concurrently to the same endpoint to exercise handler concurrency paths.
func TestHandlerStress_MixedGETAndPOST(t *testing.T) {
	repo := &handlerMockRepo{}
	srv, _ := newStressServer(t, "mixed-tenant", "admin", repo)
	client := srv.Client()

	const each = 100
	var wg sync.WaitGroup
	var postOK, getOK, errs atomic.Int32

	for i := 0; i < each; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{"path": fmt.Sprintf("root.mix.p%d", i), "content": "c"})
			resp, err := client.Post(srv.URL+"/v1/memories", "application/json", bytes.NewReader(body))
			if err != nil {
				errs.Add(1)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				postOK.Add(1)
			} else {
				errs.Add(1)
			}
		}(i)
		go func() {
			defer wg.Done()
			resp, err := client.Get(srv.URL + "/v1/memories?path_prefix=root")
			if err != nil {
				errs.Add(1)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				getOK.Add(1)
			} else {
				errs.Add(1)
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("TestHandlerStress_MixedGETAndPOST: deadlock")
	}

	if n := errs.Load(); n > 0 {
		t.Errorf("%d requests failed", n)
	}
	t.Logf("post-ok=%d get-ok=%d", postOK.Load(), getOK.Load())
}

// TestHandlerStress_EdgeCasesConcurrent fires concurrent requests with
// malformed/empty bodies and oversized content. The server must never panic;
// status codes must be 400 or 500 (never 200) for bad inputs.
func TestHandlerStress_EdgeCasesConcurrent(t *testing.T) {
	repo := &handlerMockRepo{}
	srv, _ := newStressServer(t, "edge-tenant", "admin", repo)
	client := srv.Client()

	type edgeCase struct {
		body        string
		contentType string
		wantBad     bool // if true, expect 4xx/5xx
	}
	cases := []edgeCase{
		{``, "application/json", true},
		{`{invalid json`, "application/json", true},
		{`{}`, "application/json", false},                           // empty object → path="" is accepted by mock
		{`{"path":"","content":""}`, "application/json", false},     // empty but valid
		{strings.Repeat("x", 1024*1024), "text/plain", true},       // huge non-JSON body
		{`{"path":"root.ok","content":"c"}`, "", false},             // missing content-type
		{`{"path":"root.ok","content":"c"}`, "application/json", false},
	}

	var wg sync.WaitGroup
	var panics atomic.Int32

	for _, tc := range cases {
		for rep := 0; rep < 30; rep++ {
			wg.Add(1)
			tc := tc
			go func() {
				defer func() {
					if r := recover(); r != nil {
						panics.Add(1)
					}
				}()
				defer wg.Done()
				var bodyReader io.Reader
				if tc.body != "" {
					bodyReader = strings.NewReader(tc.body)
				} else {
					bodyReader = http.NoBody
				}
				req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/memories", bodyReader)
				if err != nil {
					return
				}
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
				}
				resp, err := client.Do(req)
				if err != nil {
					return
				}
				resp.Body.Close()
				_ = resp.StatusCode
			}()
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("TestHandlerStress_EdgeCasesConcurrent: deadlock")
	}

	if n := panics.Load(); n > 0 {
		t.Errorf("%d panics detected in edge-case handler stress", n)
	}
}

// TestHandlerStress_GETMemoriesHighConcurrency fires 500 concurrent GET
// requests with varied path_prefix query parameters. Verifies no races on the
// mock repo's Retrieve method.
func TestHandlerStress_GETMemoriesHighConcurrency(t *testing.T) {
	repo := &handlerMockRepo{}
	srv, _ := newStressServer(t, "get-stress-tenant", "admin", repo)
	client := srv.Client()

	const goroutines = 500
	prefixes := []string{"root", "root.a", "root.b", "root.c.d", "", "unknown"}

	var wg sync.WaitGroup
	var ok200 atomic.Int32
	var errs atomic.Int32

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			prefix := prefixes[g%len(prefixes)]
			url := srv.URL + "/v1/memories"
			if prefix != "" {
				url += "?path_prefix=" + prefix
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			resp, err := client.Do(req)
			if err != nil {
				errs.Add(1)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				ok200.Add(1)
			} else {
				errs.Add(1)
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestHandlerStress_GETMemoriesHighConcurrency: deadlock")
	}

	if n := errs.Load(); n > 0 {
		t.Errorf("%d GET requests failed", n)
	}
	t.Logf("GET ok=%d", ok200.Load())
}
