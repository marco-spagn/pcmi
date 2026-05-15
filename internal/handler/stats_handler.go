package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/middleware"
	"github.com/marco-spagn/pcmi/internal/repository"
)

func RegisterStatsRoute(api fiber.Router, dbWrite, readReplica *pgxpool.Pool) {
	repo := repository.NewStatsRepository(dbWrite, readReplica)
	api.Get("/stats", func(c *fiber.Ctx) error {
		tenantID := c.Locals(middleware.TenantContextKey).(string)
		stats, err := repo.TenantStats(c.Context(), tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(stats)
	})
}
