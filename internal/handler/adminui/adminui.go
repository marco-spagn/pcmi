package adminui

import (
	_ "embed"

	"github.com/gofiber/fiber/v2"
)

//go:embed adminui.html
var adminHTML []byte

// Register mounts GET /ui on the given router (use under /v1/admin).
func Register(r fiber.Router) {
	r.Get("/ui", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.Send(adminHTML)
	})
}
