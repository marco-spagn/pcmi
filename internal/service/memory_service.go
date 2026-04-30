package service

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/repository"
)

type MemoryService struct {
	repo *repository.MemoryRepository
}

func NewMemoryService(db *pgxpool.Pool) *MemoryService {
	return &MemoryService{repo: repository.NewMemoryRepository(db)}
}

func (s *MemoryService) Store(c *fiber.Ctx) error {
	// TODO: implement full store with embedding + event
	return c.JSON(fiber.Map{"status": "stored", "id": 1})
}

func (s *MemoryService) Retrieve(c *fiber.Ctx) error {
	// TODO: implement hybrid retrieval (ltree + vector)
	return c.JSON([]interface{}{})
}
