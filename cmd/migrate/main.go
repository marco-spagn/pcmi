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
	"github.com/marco-spagn/pcmi/internal/config"
)

func main() {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	n, skipped, err := runMigrations(ctx, pool, cfg.MigrationsDir)
	if err != nil {
		log.Fatalf(" migrations: %v", err)
	}
	fmt.Printf("\n Migrations: %d applied, %d skipped.\n", n, skipped)
}

// runMigrations applies all .sql files in dir that haven't been applied yet.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) (applied, skipped int, err error) {
	if _, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return 0, 0, fmt.Errorf("create schema_migrations: %w", err)
	}

	rows, err := pool.Query(ctx,
		`SELECT filename FROM schema_migrations ORDER BY filename`)
	if err != nil {
		return 0, 0, fmt.Errorf("query applied: %w", err)
	}
	alreadyApplied := map[string]bool{}
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		alreadyApplied[name] = true
	}
	rows.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, fmt.Errorf("read dir %q: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, fname := range files {
		if alreadyApplied[fname] {
			log.Printf("⏭  skip  %s (already applied)", fname)
			skipped++
			continue
		}
		sql, err := os.ReadFile(filepath.Join(dir, fname))
		if err != nil {
			return applied, skipped, fmt.Errorf("read %s: %w", fname, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return applied, skipped, fmt.Errorf("begin tx for %s: %w", fname, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return applied, skipped, fmt.Errorf("apply %s: %w", fname, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, fname); err != nil {
			_ = tx.Rollback(ctx)
			return applied, skipped, fmt.Errorf("record %s: %w", fname, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return applied, skipped, fmt.Errorf("commit %s: %w", fname, err)
		}
		log.Printf("applied %s", fname)
		applied++
	}
	return applied, skipped, nil
}
