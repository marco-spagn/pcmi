package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDistillationPolicyHandler_Create_invalidBody(t *testing.T) {
	h := &DistillationPolicyHandler{}
	app := fiber.New()
	app.Post("/v1/distillation/policies", h.Create)

	req := httptest.NewRequest("POST", "/v1/distillation/policies", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 && resp.StatusCode != 400 {
		t.Fatalf("status=%d want 401 or 400", resp.StatusCode)
	}
}
