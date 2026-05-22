package config

import (
	"strings"
	"testing"
)

func TestLoad_eventBackendDefault(t *testing.T) {
	t.Setenv("EVENT_BACKEND", "")
	cfg := Load()
	if cfg.EventBackend != "streams" {
		t.Fatalf("got %q", cfg.EventBackend)
	}
}

func TestValidate_rateLimitWindowZeroAllowed(t *testing.T) {
	cfg := &Config{
		DatabaseURL:            "postgres://localhost/pcmi",
		DistillationBatchSize:  10,
		DistillationConcurrency: 4,
		PruneRetentionDays:     30,
		PruneIntervalSecs:      3600,
		ExpiryIntervalSecs:     3600,
		WebhookMaxAttempts:     5,
		RateLimitRPM:           120,
		RateLimitWindowSecs:    0,
		RateLimitMaxRequests:   0,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidate_rateLimitBackendInvalid(t *testing.T) {
	cfg := &Config{
		DatabaseURL:            "postgres://localhost/pcmi",
		DistillationBatchSize:  10,
		DistillationConcurrency: 4,
		PruneRetentionDays:     30,
		PruneIntervalSecs:      3600,
		ExpiryIntervalSecs:     3600,
		WebhookMaxAttempts:     5,
		RateLimitRPM:           120,
		RateLimitBackend:       "kafka",
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "RATE_LIMIT_BACKEND") {
		t.Fatalf("got err=%v", err)
	}
}
