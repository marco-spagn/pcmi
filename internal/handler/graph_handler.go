package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/graph"
	"github.com/marco-spagn/pcmi/internal/middleware"
)

// GraphHandler exposes the v3.0 Cognitive Graph endpoints.
// EXPERIMENTAL — requires Apache AGE PostgreSQL extension.
type GraphHandler struct {
	client *graph.GraphClient
}

func NewGraphHandler(client *graph.GraphClient) *GraphHandler {
	return &GraphHandler{client: client}
}

// RegisterGraphRoutes registers graph endpoints on app only when graphClient is
// non-nil.  Both routes are always registered; unavailability is reported at
// request time so callers can detect the feature flag status.
func RegisterGraphRoutes(app *fiber.App, graphClient *graph.GraphClient) {
	if graphClient == nil {
		return
	}
	h := NewGraphHandler(graphClient)
	app.Get("/v1/graph/health", h.Health)
	app.Get("/v1/graph/related", h.FindRelated)
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
// GET /v1/graph/related?memory_id=123&depth=3&link_types=causal,temporal
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

	related, err := h.client.FindRelated(c.Context(), tenantID, memID, linkTypes, depth)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if related == nil {
		related = []graph.RelatedMemory{}
	}
	return c.JSON(fiber.Map{
		"memory_id": memID,
		"depth":     depth,
		"entries":   related,
		"count":     len(related),
	})
}
