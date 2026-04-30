package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func New(url string) *pgxpool.Pool {
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		panic(err)
	}
	return pool
}
