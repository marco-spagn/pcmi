package service

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/model"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type MemoryService struct {
	repo *repository.MemoryRepository
}

func NewMemoryService(db *pgxpool.Pool) *MemoryService {
	return &MemoryService{repo: repository.NewMemoryRepository(db)}
}

func (s *MemoryService) Store(c *fiber.Ctx) error {
	var req model.StoreRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	id, err := s.repo.Store(c.Context(), req)
	if err != nil {
		log.Printf("Store error: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error":   "failed to store memory",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"status": "stored", "id": id})
}

func (s *MemoryService) Retrieve(c *fiber.Ctx) error {
	var req model.RetrieveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	entries, err := s.repo.Retrieve(c.Context(), req)
	if err != nil {
		log.Printf("Retrieve error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(model.RetrieveResponse{
		Entries: entries,
		Total:   len(entries),
	})
}
