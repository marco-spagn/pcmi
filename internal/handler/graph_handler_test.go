package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/graph"
)

// fakeGraphClient implements graphClientIface for handler tests (no DB needed).
type fakeGraphClient struct {
	available bool
	related   []graph.RelatedMemory
	findErr   error
	lastDepth int
}

func (f *fakeGraphClient) IsAvailable(_ context.Context) bool { return f.available }

func (f *fakeGraphClient) FindRelated(_ context.Context, _ string, _ int64, _ []string, depth int) ([]graph.RelatedMemory, error) {
	f.lastDepth = depth
	return f.related, f.findErr
}

func newGraphApp(tenantID, role string, fake graphClientIface) *fiber.App {
	app := newTestApp(tenantID, role)
	h := &GraphHandler{client: fake}
	app.Get("/v1/graph/health", h.Health)
	app.Get("/v1/graph/related", h.FindRelated)
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
	resp, err := newGraphApp("ten-1", "user", &fakeGraphClient{available: true, related: []graph.RelatedMemory{}}).
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
	if body.Entries == nil {
		t.Error("entries must be non-nil empty array, not null")
	}
}

func TestGraphHandler_FindRelated_WithResults(t *testing.T) {
	related := []graph.RelatedMemory{
		{ID: 7, LinkType: graph.LinkTypeCausal, Depth: 1},
		{ID: 15, LinkType: graph.LinkTypeTemporal, Depth: 2},
	}
	resp, err := newGraphApp("ten-1", "user", &fakeGraphClient{available: true, related: related}).
		Test(httptest.NewRequest("GET", "/v1/graph/related?memory_id=1&depth=3&link_types=causal,temporal", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d want 200", resp.StatusCode)
	}
	var body struct {
		Count   int                  `json:"count"`
		Entries []graph.RelatedMemory `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Count != 2 {
		t.Errorf("count: got %d want 2", body.Count)
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
	resp, err := newGraphApp("tid", "readonly", &fakeGraphClient{available: true, related: related}).
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

// ─── RegisterGraphRoutes ─────────────────────────────────────────────────────

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
