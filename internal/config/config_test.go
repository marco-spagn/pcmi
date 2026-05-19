package config

import (
	"strings"
	"testing"
)

// ─── Load defaults ────────────────────────────────────────────────────────────

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("API_PORT", "")
	t.Setenv("GRPC_PORT", "")

	cfg := Load()
	if cfg.RedisAddr != "redis:6379" {
		t.Errorf("RedisAddr default: got %q", cfg.RedisAddr)
	}
	if cfg.APIPort != "8000" {
		t.Errorf("APIPort default: got %q", cfg.APIPort)
	}
	if cfg.GRPCPort != "50051" {
		t.Errorf("GRPCPort default: got %q", cfg.GRPCPort)
	}
	if cfg.DistillationBatchSize != 10 {
		t.Errorf("DistillationBatchSize default: got %d", cfg.DistillationBatchSize)
	}
	if cfg.RateLimitRPM != 120 {
		t.Errorf("RateLimitRPM default: got %d", cfg.RateLimitRPM)
	}
}

// ─── TLS fields ───────────────────────────────────────────────────────────────

func TestLoadTLSEmpty(t *testing.T) {
	t.Setenv("PCMI_TLS_CERT", "")
	t.Setenv("PCMI_TLS_KEY", "")
	cfg := Load()
	if cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" {
		t.Error("expected TLS fields to be empty when env vars are unset")
	}
}

func TestLoadTLSSet(t *testing.T) {
	t.Setenv("PCMI_TLS_CERT", "/etc/certs/server.crt")
	t.Setenv("PCMI_TLS_KEY", "/etc/certs/server.key")
	cfg := Load()
	if cfg.TLSCertFile != "/etc/certs/server.crt" {
		t.Errorf("TLSCertFile: got %q", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/etc/certs/server.key" {
		t.Errorf("TLSKeyFile: got %q", cfg.TLSKeyFile)
	}
}

func TestLoadTLSTrimsWhitespace(t *testing.T) {
	t.Setenv("PCMI_TLS_CERT", "  /path/cert.pem  ")
	t.Setenv("PCMI_TLS_KEY", " /path/key.pem ")
	cfg := Load()
	if cfg.TLSCertFile != "/path/cert.pem" {
		t.Errorf("TLSCertFile not trimmed: got %q", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/path/key.pem" {
		t.Errorf("TLSKeyFile not trimmed: got %q", cfg.TLSKeyFile)
	}
}

// ─── Per-role rate limit fields ───────────────────────────────────────────────

func TestLoadRateLimitPerRoleDefaults(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM_ADMIN", "")
	t.Setenv("RATE_LIMIT_RPM_WRITE", "")
	t.Setenv("RATE_LIMIT_RPM_READONLY", "")
	cfg := Load()
	if cfg.RateLimitRPMAdmin != 30 {
		t.Errorf("RateLimitRPMAdmin default: got %d", cfg.RateLimitRPMAdmin)
	}
	if cfg.RateLimitRPMWrite != 100 {
		t.Errorf("RateLimitRPMWrite default: got %d", cfg.RateLimitRPMWrite)
	}
	if cfg.RateLimitRPMReadonly != 200 {
		t.Errorf("RateLimitRPMReadonly default: got %d", cfg.RateLimitRPMReadonly)
	}
}

func TestLoadRateLimitPerRoleOverride(t *testing.T) {
	t.Setenv("RATE_LIMIT_RPM_ADMIN", "15")
	t.Setenv("RATE_LIMIT_RPM_WRITE", "50")
	t.Setenv("RATE_LIMIT_RPM_READONLY", "300")
	cfg := Load()
	if cfg.RateLimitRPMAdmin != 15 {
		t.Errorf("expected 15, got %d", cfg.RateLimitRPMAdmin)
	}
	if cfg.RateLimitRPMWrite != 50 {
		t.Errorf("expected 50, got %d", cfg.RateLimitRPMWrite)
	}
	if cfg.RateLimitRPMReadonly != 300 {
		t.Errorf("expected 300, got %d", cfg.RateLimitRPMReadonly)
	}
}

// ─── DistillationConcurrency ──────────────────────────────────────────────────

func TestLoadDistillationConcurrencyDefault(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "")
	cfg := Load()
	if cfg.DistillationConcurrency != 4 {
		t.Errorf("expected default 4, got %d", cfg.DistillationConcurrency)
	}
}

func TestLoadDistillationConcurrencyOverride(t *testing.T) {
	t.Setenv("DISTILLATION_CONCURRENCY", "8")
	cfg := Load()
	if cfg.DistillationConcurrency != 8 {
		t.Errorf("expected 8, got %d", cfg.DistillationConcurrency)
	}
}

func TestValidateDistillationConcurrencyOutOfRange(t *testing.T) {
	cfg := &Config{
		DatabaseURL:             "postgres://x",
		DistillationBatchSize:   10,
		DistillationConcurrency: 0, // invalid
		PruneRetentionDays:      30,
		PruneIntervalSecs:       3600,
		ExpiryIntervalSecs:      3600,
		WebhookMaxAttempts:      5,
		RateLimitRPM:            60,
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DISTILLATION_CONCURRENCY") {
		t.Fatalf("expected DISTILLATION_CONCURRENCY error, got: %v", err)
	}
}

// ─── Validate ─────────────────────────────────────────────────────────────────

func TestValidateMissingDatabaseURL(t *testing.T) {
	cfg := &Config{
		DatabaseURL:             "",
		DistillationBatchSize:   10,
		DistillationConcurrency: 4,
		PruneRetentionDays:      30,
		PruneIntervalSecs:       3600,
		ExpiryIntervalSecs:      3600,
		WebhookMaxAttempts:      5,
		RateLimitRPM:            60,
	}
	err := cfg.Validate(RequireDatabaseURL)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got: %v", err)
	}
}

func TestValidateOutOfRangeBatchSize(t *testing.T) {
	cfg := &Config{
		DatabaseURL:             "postgres://x",
		DistillationBatchSize:   9999,
		DistillationConcurrency: 4,
		PruneRetentionDays:      30,
		PruneIntervalSecs:       3600,
		ExpiryIntervalSecs:      3600,
		WebhookMaxAttempts:      5,
		RateLimitRPM:            60,
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DISTILLATION_BATCH_SIZE") {
		t.Fatalf("expected batch size error, got: %v", err)
	}
}

func TestValidateValid(t *testing.T) {
	cfg := &Config{
		DatabaseURL:             "postgres://x",
		DistillationBatchSize:   10,
		DistillationConcurrency: 4,
		PruneRetentionDays:      30,
		PruneIntervalSecs:       3600,
		ExpiryIntervalSecs:      3600,
		WebhookMaxAttempts:      5,
		RateLimitRPM:            60,
	}
	if err := cfg.Validate(RequireDatabaseURL); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	cfg := &Config{
		DatabaseURL:           "",
		DistillationBatchSize: 0, // invalid
		PruneRetentionDays:    0, // invalid
		PruneIntervalSecs:     0, // invalid
		ExpiryIntervalSecs:    0, // invalid
		WebhookMaxAttempts:    0, // invalid
		RateLimitRPM:          0, // invalid
	}
	err := cfg.Validate(RequireDatabaseURL)
	if err == nil {
		t.Fatal("expected multiple validation errors")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Error("expected DATABASE_URL in errors")
	}
}
