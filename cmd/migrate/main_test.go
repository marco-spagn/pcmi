//go:build integration

package main

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunMigrations_AllAlreadyApplied(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	dir := findMigrationsDir(t)

	// Seed the tracking table with all migration files to simulate already-applied state
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// Pre-seed tracking table
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		_, _ = pool.Exec(ctx, `INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`, f)
	}

	// Now run: should apply 0 new migrations
	n, skipped, err := runMigrations(ctx, pool, dir)
	if err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 applied, got %d (skipped=%d)", n, skipped)
	}
}

func TestRunMigrations_BadDir(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	_, _, err = runMigrations(ctx, pool, "/nonexistent/dir")
	if err == nil {
		t.Fatal("expected error for bad directory")
	}
}

func findMigrationsDir(t *testing.T) string {
	t.Helper()
	for _, d := range []string{"../../migrations", "migrations"} {
		if _, err := os.Stat(d); err == nil {
			return d
		}
	}
	t.Fatal("cannot find migrations directory")
	return ""
}
