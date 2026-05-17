//go:build integration

package config_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/marco-spagn/pcmi/internal/config"
)

// Integration tests require a running PostgreSQL reachable via DATABASE_URL.
// Run with:
//
//	DATABASE_URL="postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable" \
//	  go test -tags=integration ./internal/config/...

// TestIntegration_LoadAndValidate verifies that Load()+Validate() passes when
// DATABASE_URL is set and the database is reachable.
func TestIntegration_LoadAndValidate(t *testing.T) {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	if err := cfg.Validate(config.APIRequiredFields...); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}
}

// TestIntegration_DatabaseConnectivity opens a real pgxpool connection using the
// DatabaseURL from Config and confirms a simple query succeeds.
func TestIntegration_DatabaseConnectivity(t *testing.T) {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	var result int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&result); err != nil {
		t.Fatalf("ping query failed: %v", err)
	}
	if result != 1 {
		t.Errorf("expected SELECT 1 = 1, got %d", result)
	}
}

// TestIntegration_ReadReplicaOptional verifies that when DATABASE_READ_URL is
// absent, Load() still produces a valid config with an empty ReadURL and the
// primary pool is usable.
func TestIntegration_ReadReplicaOptional(t *testing.T) {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	// Read replica is optional; its absence must not cause a validation failure.
	if err := cfg.Validate(config.APIRequiredFields...); err != nil {
		t.Fatalf("config without read replica should still be valid: %v", err)
	}

	if cfg.DatabaseReadURL != "" {
		t.Logf("DATABASE_READ_URL present: %s", cfg.DatabaseReadURL[:min(len(cfg.DatabaseReadURL), 30)])
	} else {
		t.Log("DATABASE_READ_URL not set — read replica disabled (expected)")
	}
}

// TestIntegration_WorkerConfig verifies that WorkerRequiredFields pass when
// DATABASE_URL is set (worker does not require ADMIN_API_KEY or ENCRYPTION_KEY).
func TestIntegration_WorkerConfig(t *testing.T) {
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	if err := cfg.Validate(config.WorkerRequiredFields...); err != nil {
		t.Fatalf("worker config validation failed: %v", err)
	}
}

// TestIntegration_FailFastOnMissingDatabaseURL confirms that an empty
// DATABASE_URL causes Validate() to return an error containing the expected
// message (simulates what happens at service startup).
func TestIntegration_FailFastOnMissingDatabaseURL(t *testing.T) {
	cfg := &config.Config{
		DistillationBatchSize: 10,
		PruneRetentionDays:    30,
		PruneIntervalSecs:     3600,
		ExpiryIntervalSecs:    3600,
		WebhookMaxAttempts:    5,
		RateLimitRPM:          60,
	}

	err := cfg.Validate(config.RequireDatabaseURL)
	if err == nil {
		t.Fatal("expected fail-fast error for empty DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Errorf("expected DATABASE_URL message, got: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
