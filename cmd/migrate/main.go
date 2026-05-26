// cmd/migrate applies all SQL migrations in order.
// It is designed to run as a Kubernetes init-container or a one-off job
// before the API and Worker start. It is idempotent: already-applied
// migrations are skipped via a schema_migrations tracking table.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/log"
)

func main() {
	cfg := config.Load()
	addSource := cfg.LogSource == "1" || cfg.LogSource == "true"
	log.Configure(cfg.LogFormat, cfg.LogLevel, addSource)
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	migrationsDir := cfg.MigrationsDir

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("database connection failed", "err", err)
	}
	defer pool.Close()

	// Ensure tracking table exists.
	if _, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		log.Fatal("create schema_migrations table failed", "err", err)
	}

	// Read already-applied migrations.
	rows, err := pool.Query(ctx,
		`SELECT filename FROM schema_migrations ORDER BY filename`)
	if err != nil {
		log.Fatal("query applied migrations failed", "err", err)
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
		log.Fatal("read migrations directory failed", "dir", migrationsDir, "err", err)
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
			log.Info("migration already applied, skipping", "file", fname)
			continue
		}
		sql, err := os.ReadFile(filepath.Join(migrationsDir, fname))
		if err != nil {
			log.Fatal("read migration file failed", "file", fname, "err", err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Fatal("begin transaction failed", "file", fname, "err", err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatal("apply migration failed", "file", fname, "err", err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, fname); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatal("record migration failed", "file", fname, "err", err)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Fatal("commit migration failed", "file", fname, "err", err)
		}
		log.Info("applied migration", "file", fname)
		appliedCount++
	}
	fmt.Printf("\n🎉 Migrations: %d applied, %d skipped.\n",
		appliedCount, len(files)-appliedCount)
}
