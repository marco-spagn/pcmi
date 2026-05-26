package adminui

import (
	_ "embed"

	"github.com/gofiber/fiber/v2"

	"github.com/marco-spagn/pcmi/internal/middleware"
)

//go:embed adminui.html
var adminHTML []byte

// Register mounts GET /v1/admin/ui (HTML shell without X-API-Key; admin role when key sent).
func Register(app *fiber.App) {
	app.Get("/v1/admin/ui", middleware.RequireAdminRoleIfAPIKeyPresent, func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(adminHTML)
	})
}
