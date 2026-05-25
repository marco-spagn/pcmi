package adminui

import (
	_ "embed"

	"github.com/gofiber/fiber/v2"
)

//go:embed adminui.html
var adminHTML []byte

// Register mounts GET /v1/admin/ui on the app (no API key on page load).
func Register(app *fiber.App) {
	app.Get("/v1/admin/ui", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(adminHTML)
	})
}
