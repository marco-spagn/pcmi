package handler

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marco-spagn/pcmi/internal/config"
)

func TestSetupMemoryRoutes_DisabledEmbedding(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	cfg := &config.Config{
		OpenAIAPIKey:   "",
		OpenAIBaseURL:  "",
		EmbeddingModel: "text-embedding-3-small",
	}
	// nil pools: only NewFromConfig path is exercised before route registration would touch DB.
	if err := SetupMemoryRoutes(app, nil, nil, cfg); err != nil {
		t.Fatalf("SetupMemoryRoutes with empty API key: %v", err)
	}
}
