// cmd/migrate applies all SQL migrations in order.
// It is designed to run as a Kubernetes init-container or a one-off job
// before the API and Worker start. It is idempotent: already-applied
// migrations are skipped via a schema_migrations tracking table.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// Ensure tracking table exists.
	if _, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		log.Fatalf("create schema_migrations: %v", err)
	}

	// Read already-applied migrations.
	rows, err := pool.Query(ctx,
		`SELECT filename FROM schema_migrations ORDER BY filename`)
	if err != nil {
		log.Fatalf("query applied: %v", err)
	}
	applied := map[string]bool{}
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		applied[name] = true
	}
	rows.Close()

	// Collect .sql files in lexicographic order.
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		log.Fatalf("read migrations dir %q: %v", migrationsDir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	appliedCount := 0
	for _, fname := range files {
		if applied[fname] {
			log.Printf("⏭  skip  %s (already applied)", fname)
			continue
		}
		sql, err := os.ReadFile(filepath.Join(migrationsDir, fname))
		if err != nil {
			log.Fatalf("read %s: %v", fname, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Fatalf("begin tx for %s: %v", fname, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("❌ apply %s: %v", fname, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, fname); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("record %s: %v", fname, err)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Fatalf("commit %s: %v", fname, err)
		}
		log.Printf("✅ applied %s", fname)
		appliedCount++
	}
	fmt.Printf("\n🎉 Migrations: %d applied, %d skipped.\n",
		appliedCount, len(files)-appliedCount)
}
