package handler

import (
	_ "embed"

	"github.com/gofiber/fiber/v2"
)

//go:embed graph_ui.html
var graphUIHTML []byte

// RegisterGraphUIRoute serves the cognitive graph visual explorer.
func RegisterGraphUIRoute(app *fiber.App) {
	app.Get("/v1/graph/ui", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(graphUIHTML)
	})
}
