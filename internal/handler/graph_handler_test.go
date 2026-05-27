package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

// fakeGraphClient implements graphClientIface for handler tests (no DB needed).
type fakeGraphClient struct {
	available  bool
	related    []graph.RelatedMemory
	relatedTot int
	findErr    error
	lastDepth  int
	lastOffset int
	lastLimit  int

	chainResult *graph.ChainResult
	chainErr    error

	cypherResult *graph.CypherResult
	cypherErr    error
}

func (f *fakeGraphClient) IsAvailable(_ context.Context) bool { return f.available }

func (f *fakeGraphClient) FindRelated(_ context.Context, _ string, _ int64, _ []string, depth, offset, limit int) (*graph.RelatedResult, error) {
	f.lastDepth = depth
	f.lastOffset = offset
	f.lastLimit = limit
	return &graph.RelatedResult{Memories: f.related, Total: f.relatedTot}, f.findErr
}

func (f *fakeGraphClient) FindChain(_ context.Context, _ string, _, _ int64, _ []string, _ int) (*graph.ChainResult, error) {
	if f.chainResult == nil {
		return &graph.ChainResult{}, f.chainErr
	}
	return f.chainResult, f.chainErr
}

func (f *fakeGraphClient) ExecuteCypher(_ context.Context, _ string, _ string) (*graph.CypherResult, error) {
	if f.cypherResult == nil {
		return &graph.CypherResult{}, f.cypherErr
	}
	return f.cypherResult, f.cypherErr
}

func newGraphApp(tenantID, role string, fake graphClientIface) *fiber.App {
	app := newTestApp(tenantID, role)
	h := &GraphHandler{client: fake}
	app.Get("/v1/graph/health", h.Health)
	app.Get("/v1/graph/related", h.FindRelated)
	app.Get("/v1/graph/chain", h.FindChain)
	app.Post("/v1/graph/cypher", middleware.RequireWriteRole, h.ExecuteCypher)
	return app
}

// ─── Health ──────────────────────────────────────────────────────────────────

func TestGraphHandler_Health_Available(t *testing.T) {
	resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: true}).
		Test(httptest.NewRequest("GET", "/v1/graph/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["available"] != true {
		t.Errorf("available: got %v want true", body["available"])
	}
	if body["extension"] != "apache-age" {
		t.Errorf("extension: got %v want apache-age", body["extension"])
	}
}

func TestGraphHandler_Health_NotAvailable(t *testing.T) {
	resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: false}).
		Test(httptest.NewRequest("GET", "/v1/graph/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["available"] != false {
		t.Errorf("available: got %v want false", body["available"])
	}
}

// ─── FindRelated ─────────────────────────────────────────────────────────────

func TestGraphHandler_FindRelated_AGENotAvailable_Returns501(t *testing.T) {
	resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: false}).
		Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("status %d want 501", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "cognitive graph not available" {
		t.Errorf("error field: %v", body["error"])
	}
	if _, ok := body["hint"]; !ok {
		t.Error("response must include a hint field")
	}
}

func TestGraphHandler_FindRelated_MissingMemoryID_Returns400(t *testing.T) {
	resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: true}).
		Test(httptest.NewRequest("GET", "/v1/graph/related", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestGraphHandler_FindRelated_InvalidMemoryID_Returns400(t *testing.T) {
	resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: true}).
		Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=abc", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestGraphHandler_FindRelated_EmptyResults_ReturnsArray(t *testing.T) {
	resp, err := newGraphApp("ten-1", "user", &fakeGraphClient{available: true, related: []graph.RelatedMemory{}, relatedTot: 0}).
		Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=42&depth=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		MemoryID float64              `json:"memory_id"`
		Depth    int                  `json:"depth"`
		Count    int                  `json:"count"`
		Total    int                  `json:"total"`
		Offset   int                  `json:"offset"`
		Limit    int                  `json:"limit"`
		Entries  []graph.RelatedMemory `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.MemoryID != 42 {
		t.Errorf("memory_id: got %v want 42", body.MemoryID)
	}
	if body.Depth != 2 {
		t.Errorf("depth: got %d want 2", body.Depth)
	}
	if body.Count != 0 {
		t.Errorf("count: got %d want 0", body.Count)
	}
	if body.Total != 0 {
		t.Errorf("total: got %d want 0", body.Total)
	}
	if body.Entries == nil {
		t.Error("entries must be non-nil empty array, not null")
	}
}

func TestGraphHandler_FindRelated_WithResults(t *testing.T) {
	related := []graph.RelatedMemory{
		{ID: 7, LinkType: graph.LinkTypeCausal, Depth: 1},
		{ID: 15, LinkType: graph.LinkTypeTemporal, Depth: 2},
	}
	resp, err := newGraphApp("ten-1", "user", &fakeGraphClient{available: true, related: related, relatedTot: 2}).
		Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1&depth=3&link_types=causal,temporal", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Count   int                   `json:"count"`
		Total   int                   `json:"total"`
		Entries []graph.RelatedMemory `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 2 {
		t.Errorf("count: got %d want 2", body.Count)
	}
	if body.Total != 2 {
		t.Errorf("total: got %d want 2", body.Total)
	}
	if len(body.Entries) != 2 {
		t.Errorf("entries len: got %d want 2", len(body.Entries))
	}
}

func TestGraphHandler_FindRelated_DepthDefaultsTo3(t *testing.T) {
	fake := &fakeGraphClient{available: true, related: []graph.RelatedMemory{}}
	app := newGraphApp("tid", "user", fake)
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if fake.lastDepth != 3 {
		t.Errorf("default depth: got %d want 3", fake.lastDepth)
	}
}

func TestGraphHandler_FindRelated_DepthClamped_TooLarge(t *testing.T) {
	fake := &fakeGraphClient{available: true, related: []graph.RelatedMemory{}}
	app := newGraphApp("tid", "user", fake)
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1&depth=99", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if fake.lastDepth != 3 {
		t.Errorf("depth 99 should use default 3; got %d", fake.lastDepth)
	}
}

func TestGraphHandler_FindRelated_DepthClamped_Zero(t *testing.T) {
	fake := &fakeGraphClient{available: true, related: []graph.RelatedMemory{}}
	app := newGraphApp("tid", "user", fake)
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1&depth=0", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if fake.lastDepth != 3 {
		t.Errorf("depth 0 should use default 3; got %d", fake.lastDepth)
	}
}

func TestGraphHandler_FindRelated_ReadonlyRoleAllowed(t *testing.T) {
	related := []graph.RelatedMemory{{ID: 1, LinkType: graph.LinkTypeRelated, Depth: 1}}
	resp, err := newGraphApp("tid", "readonly", &fakeGraphClient{available: true, related: related, relatedTot: 1}).
		Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("readonly role must be allowed on GET graph/related; status %d", resp.StatusCode)
	}
}

func TestGraphHandler_FindRelated_InternalError(t *testing.T) {
	fake := &fakeGraphClient{available: true, findErr: errors.New("graph traversal failed")}
	resp, err := newGraphApp("tid", "user", fake).
		Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == nil {
		t.Error("error field must be present on 500")
	}
}

func TestGraphHandler_FindRelated_PaginationParams(t *testing.T) {
	related := []graph.RelatedMemory{
		{ID: 7, LinkType: graph.LinkTypeCausal, Depth: 1},
		{ID: 8, LinkType: graph.LinkTypeCausal, Depth: 1},
	}
	fake := &fakeGraphClient{available: true, related: related, relatedTot: 25}
	app := newGraphApp("tid", "user", fake)
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1&offset=10&limit=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	if fake.lastOffset != 10 {
		t.Errorf("offset: got %d want 10", fake.lastOffset)
	}
	if fake.lastLimit != 2 {
		t.Errorf("limit: got %d want 2", fake.lastLimit)
	}
	var body struct {
		Total  int `json:"total"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 25 {
		t.Errorf("total: got %d want 25", body.Total)
	}
	if body.Offset != 10 {
		t.Errorf("offset in response: got %d want 10", body.Offset)
	}
	if body.Limit != 2 {
		t.Errorf("limit in response: got %d want 2", body.Limit)
	}
}

// ─── FindChain ───────────────────────────────────────────────────────────────

func TestGraphHandler_FindChain_MissingParams_Returns400(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"no from", "/v1/graph/chain?to=2"},
		{"no to", "/v1/graph/chain?from=1"},
		{"neither", "/v1/graph/chain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: true}).
				Test(httptest.NewRequest("GET", tt.url, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 400 {
				t.Errorf("status %d want 400", resp.StatusCode)
			}
		})
	}
}

func TestGraphHandler_FindChain_InvalidIDs_Returns400(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"from not int", "/v1/graph/chain?from=abc&to=2"},
		{"to not int", "/v1/graph/chain?from=1&to=xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: true}).
				Test(httptest.NewRequest("GET", tt.url, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 400 {
				t.Errorf("status %d want 400", resp.StatusCode)
			}
		})
	}
}

func TestGraphHandler_FindChain_AGENotAvailable_Returns501(t *testing.T) {
	resp, err := newGraphApp("tid", "user", &fakeGraphClient{available: false}).
		Test(httptest.NewRequest("GET", "/v1/graph/chain?from=1&to=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("status %d want 501", resp.StatusCode)
	}
}

func TestGraphHandler_FindChain_Connected(t *testing.T) {
	chainResult := &graph.ChainResult{
		FromID:    1,
		ToID:      42,
		Connected: true,
		Hops:      3,
		Path: []graph.ChainLink{
			{FromID: 1, ToID: 7, LinkType: graph.LinkTypeCausal, HopIndex: 0},
			{FromID: 7, ToID: 15, LinkType: graph.LinkTypeCausal, HopIndex: 1},
			{FromID: 15, ToID: 42, LinkType: graph.LinkTypeCausal, HopIndex: 2},
		},
	}
	resp, err := newGraphApp("ten-1", "user", &fakeGraphClient{available: true, chainResult: chainResult}).
		Test(httptest.NewRequest("GET", "/v1/graph/chain?from=1&to=42&link_types=causal&max_depth=10", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body graph.ChainResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Connected {
		t.Error("expected connected=true")
	}
	if body.Hops != 3 {
		t.Errorf("hops: got %d want 3", body.Hops)
	}
	if len(body.Path) != 3 {
		t.Errorf("path length: got %d want 3", len(body.Path))
	}
}

func TestGraphHandler_FindChain_NotConnected(t *testing.T) {
	chainResult := &graph.ChainResult{
		FromID:    1,
		ToID:      99,
		Connected: false,
	}
	resp, err := newGraphApp("ten-1", "user", &fakeGraphClient{available: true, chainResult: chainResult}).
		Test(httptest.NewRequest("GET", "/v1/graph/chain?from=1&to=99", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body graph.ChainResult
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Connected {
		t.Error("expected connected=false")
	}
	if body.Path == nil {
		t.Error("path must be non-nil empty array")
	}
}

func TestGraphHandler_FindChain_ReadonlyAllowed(t *testing.T) {
	chainResult := &graph.ChainResult{FromID: 1, ToID: 2, Connected: false}
	resp, err := newGraphApp("tid", "readonly", &fakeGraphClient{available: true, chainResult: chainResult}).
		Test(httptest.NewRequest("GET", "/v1/graph/chain?from=1&to=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("readonly must be allowed on GET graph/chain; status %d", resp.StatusCode)
	}
}

func TestGraphHandler_FindChain_InternalError(t *testing.T) {
	fake := &fakeGraphClient{available: true, chainErr: errors.New("chain traversal failed")}
	resp, err := newGraphApp("tid", "user", fake).
		Test(httptest.NewRequest("GET", "/v1/graph/chain?from=1&to=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status %d want 500", resp.StatusCode)
	}
}

// ─── ExecuteCypher ───────────────────────────────────────────────────────────

func TestGraphHandler_ExecuteCypher_MissingQuery_Returns400(t *testing.T) {
	resp, err := newGraphApp("tid", "write", &fakeGraphClient{available: true}).
		Test(httptest.NewRequest("POST", "/v1/graph/cypher", strings.NewReader(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

func TestGraphHandler_ExecuteCypher_AGENotAvailable_Returns501(t *testing.T) {
	body := strings.NewReader(`{"query": "MATCH (n) RETURN n LIMIT 1"}`)
	req := httptest.NewRequest("POST", "/v1/graph/cypher", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := newGraphApp("tid", "write", &fakeGraphClient{available: false}).Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("status %d want 501", resp.StatusCode)
	}
}

func TestGraphHandler_ExecuteCypher_Success(t *testing.T) {
	cypherResult := &graph.CypherResult{
		Columns: []string{"result"},
		Rows: []map[string]interface{}{
			{"result": map[string]interface{}{"id": "memory.1"}},
		},
	}
	fake := &fakeGraphClient{available: true, cypherResult: cypherResult}
	reqBody := strings.NewReader(`{"query": "MATCH (n:Memory) RETURN n.id LIMIT 1"}`)
	req := httptest.NewRequest("POST", "/v1/graph/cypher", reqBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := newGraphApp("tid", "write", fake).Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var result graph.CypherResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Errorf("rows: got %d want 1", len(result.Rows))
	}
}

func TestGraphHandler_ExecuteCypher_ReadonlyRole_Returns403(t *testing.T) {
	reqBody := strings.NewReader(`{"query": "MATCH (n) RETURN n LIMIT 1"}`)
	req := httptest.NewRequest("POST", "/v1/graph/cypher", reqBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := newGraphApp("tid", "readonly", &fakeGraphClient{available: true}).Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("readonly must be blocked on POST graph/cypher; status %d", resp.StatusCode)
	}
}

func TestGraphHandler_ExecuteCypher_ClientError(t *testing.T) {
	fake := &fakeGraphClient{available: true, cypherErr: errors.New("invalid Cypher syntax")}
	reqBody := strings.NewReader(`{"query": "BAD QUERY"}`)
	req := httptest.NewRequest("POST", "/v1/graph/cypher", reqBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := newGraphApp("tid", "write", fake).Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status %d want 400", resp.StatusCode)
	}
}

// ─── RegisterGraphRoutes ─────────────────────────────────────────────────────

func TestRegisterGraphRoutes_UnavailableClient_RoutesRespond(t *testing.T) {
	app := fiber.New()
	RegisterGraphRoutes(app, graph.NewGraphClient(nil))
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("health status %d want 200", resp.StatusCode)
	}
	resp, err = app.Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("related without AGE: status %d want 501", resp.StatusCode)
	}
}

func TestRegisterGraphRoutes_NilClient_NoRoutes(t *testing.T) {
	app := fiber.New()
	RegisterGraphRoutes(app, nil)

	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("nil graphClient should not register routes; status %d", resp.StatusCode)
	}
}

// ─── Chain endpoint registered ───────────────────────────────────────────────

func TestRegisterGraphRoutes_ChainRouteRegistered(t *testing.T) {
	app := fiber.New()
	RegisterGraphRoutes(app, graph.NewGraphClient(nil))

	// Chain endpoint must be registered (returns 501 when AGE absent).
	resp, err := app.Test(httptest.NewRequest("GET", "/v1/graph/chain?from=1&to=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("chain without AGE: status %d want 501", resp.StatusCode)
	}
}

func TestRegisterGraphRoutes_CypherRouteRegistered(t *testing.T) {
	app := fiber.New()
	// Use NewTestApp-style locals injection for the route to work with RequireWriteRole.
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("tenant_id", "tid")
		c.Locals("role", "write")
		return c.Next()
	})
	RegisterGraphRoutes(app, graph.NewGraphClient(nil))

	cypherReq := httptest.NewRequest("POST", "/v1/graph/cypher", strings.NewReader(`{"query":"MATCH (n) RETURN n"}`))
	cypherReq.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(cypherReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNotImplemented {
		t.Fatalf("cypher without AGE: status %d want 501", resp.StatusCode)
	}
}
