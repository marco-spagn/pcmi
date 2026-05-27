package handler

import (
	"context"
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
	FindRelated(ctx context.Context, tenantID string, memoryID int64, linkTypes []string, maxDepth, offset, limit int) (*graph.RelatedResult, error)
	FindChain(ctx context.Context, tenantID string, fromID, toID int64, linkTypes []string, maxDepth int) (*graph.ChainResult, error)
	ExecuteCypher(ctx context.Context, tenantID, query string) (*graph.CypherResult, error)
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
	app.Post("/v1/graph/cypher", middleware.RequireWriteRole, h.ExecuteCypher)
}

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
// GET /v1/graph/related?memory_id=123&depth=3&link_types=causal,temporal&offset=0&limit=50
func (h *GraphHandler) FindRelated(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
			"error": "cognitive graph not available",
			"hint":  "requires PostgreSQL AGE extension — see docs/cognitive-graph.md",
		})
	}

	tenantID, _ := c.Locals(middleware.TenantContextKey).(string)

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

	var linkTypes []string
	if lt := c.Query("link_types"); lt != "" {
		for _, t := range strings.Split(lt, ",") {
			if s := strings.TrimSpace(t); s != "" {
				linkTypes = append(linkTypes, s)
			}
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	start := time.Now()
	result, err := h.client.FindRelated(c.Context(), tenantID, memID, linkTypes, depth, offset, limit)
	elapsed := time.Since(start).Seconds()
	metrics.IncGraphTraversal()
	metrics.ObserveGraphTraversal(elapsed)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if result.Memories == nil {
		result.Memories = []graph.RelatedMemory{}
	}
	return c.JSON(fiber.Map{
		"memory_id": memID,
		"depth":     depth,
		"entries":   result.Memories,
		"count":     len(result.Memories),
		"total":     result.Total,
		"offset":    offset,
		"limit":     limit,
	})
}

// FindChain returns the shortest causal chain between two memories.
//
// GET /v1/graph/chain?from=1&to=42&link_types=causal&max_depth=10
func (h *GraphHandler) FindChain(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
			"error": "cognitive graph not available",
			"hint":  "requires PostgreSQL AGE extension — see docs/cognitive-graph.md",
		})
	}

	tenantID, _ := c.Locals(middleware.TenantContextKey).(string)

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

	var linkTypes []string
	if lt := c.Query("link_types"); lt != "" {
		for _, t := range strings.Split(lt, ",") {
			if s := strings.TrimSpace(t); s != "" {
				linkTypes = append(linkTypes, s)
			}
		}
	}

	start := time.Now()
	result, err := h.client.FindChain(c.Context(), tenantID, fromID, toID, linkTypes, maxDepth)
	elapsed := time.Since(start).Seconds()
	metrics.IncGraphTraversal()
	metrics.ObserveGraphTraversal(elapsed)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if result.Path == nil {
		result.Path = []graph.ChainLink{}
	}
	return c.JSON(result)
}

// ExecuteCypher runs a read-only Cypher query against pcmi_memory_graph.
// Requires write role (ability to execute arbitrary graph queries).
//
// POST /v1/graph/cypher  {"query": "MATCH (n:Memory) WHERE n.tenant_id = '...' RETURN n.id LIMIT 10"}
func (h *GraphHandler) ExecuteCypher(c *fiber.Ctx) error {
	if !h.client.IsAvailable(c.Context()) {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
			"error": "cognitive graph not available",
			"hint":  "requires PostgreSQL AGE extension — see docs/cognitive-graph.md",
		})
	}

	tenantID, _ := c.Locals(middleware.TenantContextKey).(string)

	var req struct {
		Query string `json:"query"`
	}
	if err := c.BodyParser(&req); err != nil || req.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query field is required"})
	}

	start := time.Now()
	result, err := h.client.ExecuteCypher(c.Context(), tenantID, req.Query)
	elapsed := time.Since(start).Seconds()
	metrics.IncGraphTraversal()
	metrics.ObserveGraphTraversal(elapsed)

	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if result.Rows == nil {
		result.Rows = []map[string]interface{}{}
	}
	return c.JSON(result)
}
