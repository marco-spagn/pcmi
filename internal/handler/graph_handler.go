package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

// graphClientIface is the surface of graph.GraphClient used by GraphHandler.
// Defined here so tests can inject fakes without a real pgxpool.
type graphClientIface interface {
	IsAvailable(ctx context.Context) bool
	FindRelated(ctx context.Context, tenantID string, memoryID int64, linkTypes []string, maxDepth int, cursor int64, limit int, direction graph.TraversalDirection) (*graph.RelatedResult, error)
	FindChain(ctx context.Context, tenantID string, fromID, toID int64, linkTypes []string, maxDepth int) (*graph.ChainResult, error)
	ExecuteCypher(ctx context.Context, tenantID, query string) (*graph.CypherResult, error)
	FindEntitiesForMemory(ctx context.Context, tenantID string, memoryID int64) ([]graph.EntityMention, error)
	FindMemoriesByEntity(ctx context.Context, tenantID, kind, key string, cursor int64, limit int) (*graph.EntityMemoriesResult, error)
	FindRelatedViaEntity(ctx context.Context, tenantID string, memoryID int64, cursor int64, limit int) (*graph.EntityMemoriesResult, error)
}

// GraphHandler exposes the v2.0 Cognitive Graph endpoints.
// EXPERIMENTAL — requires Apache AGE PostgreSQL extension.
type GraphHandler struct {
	client graphClientIface
}

func NewGraphHandler(client *graph.GraphClient) *GraphHandler {
	return &GraphHandler{client: client}
}

// RegisterGraphRoutes registers graph endpoints when graphClient is non-nil.
// Routes stay registered even when AGE is absent: Health reports available=false;
// traversal endpoints return 501.
func RegisterGraphRoutes(app *fiber.App, graphClient *graph.GraphClient) {
	if graphClient == nil {
		return
	}
	h := NewGraphHandler(graphClient)
	app.Get("/v1/graph/health", h.Health)
	app.Get("/v1/graph/related", h.FindRelated)
	app.Get("/v1/graph/chain", h.FindChain)
	app.Get("/v1/graph/entities/memory", h.FindEntitiesForMemory)
	app.Get("/v1/graph/entities/related", h.FindEntitiesRelated)
	app.Post("/v1/graph/cypher", middleware.RequireWriteRole, h.ExecuteCypher)

	// Graph visual explorer (no auth — same as health).
	RegisterGraphUIRoute(app)
}

// ── Shared helpers ────────────────────────────────────────────────────────────

// ageNotAvailable returns a 501 response when AGE is not installed.
func (h *GraphHandler) ageNotAvailable(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"error": "cognitive graph not available",
		"hint":  "requires PostgreSQL AGE extension — see docs/cognitive-graph.md",
	})
}

// tenantID extracts the tenant UUID from request-scoped locals.
func (h *GraphHandler) tenantID(c *fiber.Ctx) string {
	id, _ := c.Locals(middleware.TenantContextKey).(string)
	return id
}

// parseLinkTypes splits a comma-separated link_types query parameter.
func (h *GraphHandler) parseLinkTypes(c *fiber.Ctx) []string {
	raw := c.Query("link_types")
	if raw == "" {
		return nil
	}
	var out []string
	for _, t := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(t); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// timed runs fn and records graph traversal metrics.
func (h *GraphHandler) timed(c *fiber.Ctx, fn func() error) error {
	start := time.Now()
	err := fn()
	elapsed := time.Since(start).Seconds()
	metrics.IncGraphTraversal()
	metrics.ObserveGraphTraversal(elapsed)
	return err
}

// graphError maps a graph client error to an HTTP error response.
func (h *GraphHandler) graphError(c *fiber.Ctx, err error, timeoutHint string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
			"error": "graph query timed out",
			"hint":  timeoutHint,
		})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
}

// ── Endpoints ──────────────────────────────────────────────────────────────────

// Health returns AGE availability.
// GET /v1/graph/health
func (h *GraphHandler) Health(c *fiber.Ctx) error {
	available := h.client.IsAvailable(c.Context())
	return c.JSON(fiber.Map{
		"available": available,
		"extension": "apache-age",
	})
}

// FindRelated returns memories causally/semantically connected to a given
// memory within a configurable hop depth.
//
// GET /v1/graph/related?memory_id=123&depth=3&link_types=causal,temporal&direction=both&cursor=0&limit=50
func (h *GraphHandler) FindRelated(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return h.ageNotAvailable(c)
	}

	tid := h.tenantID(c)

	memIDStr := c.Query("memory_id")
	if memIDStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "memory_id is required"})
	}
	memID, err := strconv.ParseInt(memIDStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "memory_id must be an integer"})
	}

	depth := 3
	if d := c.Query("depth"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 10 {
			depth = n
		}
	}

	linkTypes := h.parseLinkTypes(c)

	cursor := int64(0)
	if cur := c.Query("cursor"); cur != "" {
		if n, err := strconv.ParseInt(cur, 10, 64); err == nil && n >= 0 {
			cursor = n
		}
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	direction, err := graph.ParseTraversalDirection(c.Query("direction"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	var result *graph.RelatedResult
	err = h.timed(c, func() error {
		result, err = h.client.FindRelated(c.Context(), tid, memID, linkTypes, depth, cursor, limit, direction)
		return err
	})
	if err != nil {
		return h.graphError(c, err, "try reducing depth or narrowing link_types")
	}
	if result.Memories == nil {
		result.Memories = []graph.RelatedMemory{}
	}
	return c.JSON(fiber.Map{
		"memory_id":   memID,
		"depth":       depth,
		"direction":   string(direction),
		"entries":     result.Memories,
		"count":       len(result.Memories),
		"total":       result.Total,
		"next_cursor": result.NextCursor,
		"limit":       limit,
	})
}

// FindChain returns the shortest causal chain between two memories.
//
// GET /v1/graph/chain?from=1&to=42&link_types=causal&max_depth=10
func (h *GraphHandler) FindChain(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return h.ageNotAvailable(c)
	}

	tid := h.tenantID(c)

	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "from and to query parameters are required"})
	}

	fromID, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "from must be an integer"})
	}
	toID, err := strconv.ParseInt(toStr, 10, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "to must be an integer"})
	}

	maxDepth := 10
	if d := c.Query("max_depth"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 20 {
			maxDepth = n
		}
	}

	linkTypes := h.parseLinkTypes(c)

	var result *graph.ChainResult
	err = h.timed(c, func() error {
		result, err = h.client.FindChain(c.Context(), tid, fromID, toID, linkTypes, maxDepth)
		return err
	})
	if err != nil {
		return h.graphError(c, err, "try reducing max_depth or narrowing link_types")
	}
	if result.Path == nil {
		result.Path = []graph.ChainLink{}
	}
	return c.JSON(result)
}

// FindEntitiesForMemory lists promoted entity vertices linked from a memory.
//
// GET /v1/graph/entities/memory?memory_id=123
func (h *GraphHandler) FindEntitiesForMemory(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return h.ageNotAvailable(c)
	}
	tid := h.tenantID(c)
	memID, err := parseGraphMemoryID(c.Query("memory_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	var entities []graph.EntityMention
	err = h.timed(c, func() error {
		entities, err = h.client.FindEntitiesForMemory(c.Context(), tid, memID)
		return err
	})
	if err != nil {
		return h.graphError(c, err, "try again later")
	}
	if entities == nil {
		entities = []graph.EntityMention{}
	}
	return c.JSON(fiber.Map{
		"memory_id": memID,
		"entities":  entities,
		"count":     len(entities),
	})
}

// FindEntitiesRelated returns memories correlated via shared entity vertices.
//
// GET /v1/graph/entities/related?kind=IPAddress&key=10.0.4.22
// GET /v1/graph/entities/related?memory_id=123
func (h *GraphHandler) FindEntitiesRelated(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return h.ageNotAvailable(c)
	}
	tid := h.tenantID(c)
	cursor, limit := parseGraphPagination(c)

	kind := strings.TrimSpace(c.Query("kind"))
	key := strings.TrimSpace(c.Query("key"))
	memIDStr := strings.TrimSpace(c.Query("memory_id"))

	var result *graph.EntityMemoriesResult
	var err error
	err = h.timed(c, func() error {
		switch {
		case kind != "" && key != "":
			result, err = h.client.FindMemoriesByEntity(c.Context(), tid, kind, key, cursor, limit)
		case memIDStr != "":
			memID, parseErr := parseGraphMemoryID(memIDStr)
			if parseErr != nil {
				err = parseErr
				return err
			}
			result, err = h.client.FindRelatedViaEntity(c.Context(), tid, memID, cursor, limit)
		default:
			err = fmt.Errorf("provide kind+key or memory_id")
		}
		return err
	})
	if err != nil {
		if err.Error() == "provide kind+key or memory_id" || strings.Contains(err.Error(), "memory_id") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return h.graphError(c, err, "try reducing limit")
	}
	if result.Memories == nil {
		result.Memories = []graph.EntityMemory{}
	}
	return c.JSON(fiber.Map{
		"kind":        kind,
		"key":         key,
		"memory_id":   memIDStr,
		"entries":     result.Memories,
		"count":       len(result.Memories),
		"total":       result.Total,
		"next_cursor": result.NextCursor,
		"limit":       limit,
	})
}

func parseGraphMemoryID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("memory_id is required")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("memory_id must be a positive integer")
	}
	return id, nil
}

func parseGraphPagination(c *fiber.Ctx) (cursor int64, limit int) {
	limit = 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if cur := c.Query("cursor"); cur != "" {
		if n, err := strconv.ParseInt(cur, 10, 64); err == nil && n >= 0 {
			cursor = n
		}
	}
	return cursor, limit
}

// ExecuteCypher runs a read-only Cypher query against pcmi_memory_graph.
// Requires write role (ability to execute arbitrary graph queries).
//
// POST /v1/graph/cypher  {"query": "MATCH (n:Memory) RETURN n.id LIMIT 10"}
func (h *GraphHandler) ExecuteCypher(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return h.ageNotAvailable(c)
	}

	tid := h.tenantID(c)

	var req struct {
		Query string `json:"query"`
	}
	if err := c.BodyParser(&req); err != nil || req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query field is required"})
	}

	var result *graph.CypherResult
	var execErr error
	execErr = h.timed(c, func() error {
		result, execErr = h.client.ExecuteCypher(c.Context(), tid, req.Query)
		return execErr
	})
	if execErr != nil {
		if errors.Is(execErr, context.DeadlineExceeded) {
			return h.graphError(c, execErr, "try simplifying the Cypher query or adding a LIMIT clause")
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": execErr.Error()})
	}
	if result.Rows == nil {
		result.Rows = []map[string]interface{}{}
	}
	return c.JSON(result)
}
