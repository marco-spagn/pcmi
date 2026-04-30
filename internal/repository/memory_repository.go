package repository

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemoryRepository struct {
	db *pgxpool.Pool
}

func NewMemoryRepository(db *pgxpool.Pool) *MemoryRepository {
	return &MemoryRepository{db: db}
}

// Store, Retrieve, etc. (implementazione completa disponibile su richiesta)
