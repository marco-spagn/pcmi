package config

import "testing"

// These tests exercise the unexported env* helpers directly. They live in the
// same package as config.go so we can call the helpers without exposing them
// to the rest of the codebase.

func TestEnvOrUsesFallbackWhenEmpty(t *testing.T) {
	t.Setenv("PCMI_TEST_ENVOR", "")
	if got := envOr("PCMI_TEST_ENVOR", "fallback"); got != "fallback" {
		t.Fatalf("envOr empty: got %q, want %q", got, "fallback")
	}
}

func TestEnvOrReturnsValueWhenSet(t *testing.T) {
	t.Setenv("PCMI_TEST_ENVOR", "value")
	if got := envOr("PCMI_TEST_ENVOR", "fallback"); got != "value" {
		t.Fatalf("envOr set: got %q, want %q", got, "value")
	}
}

func TestEnvIntFallbackWhenUnset(t *testing.T) {
	t.Setenv("PCMI_TEST_ENVINT", "")
	if got := envInt("PCMI_TEST_ENVINT", 7); got != 7 {
		t.Fatalf("envInt unset: got %d, want 7", got)
	}
}

func TestEnvIntFallbackWhenInvalid(t *testing.T) {
	// Non-numeric value must fall back instead of panicking or returning 0.
	t.Setenv("PCMI_TEST_ENVINT", "not-a-number")
	if got := envInt("PCMI_TEST_ENVINT", 11); got != 11 {
		t.Fatalf("envInt invalid: got %d, want 11", got)
	}
}

func TestEnvIntTrimsWhitespace(t *testing.T) {
	t.Setenv("PCMI_TEST_ENVINT", "   42   ")
	if got := envInt("PCMI_TEST_ENVINT", 0); got != 42 {
		t.Fatalf("envInt whitespace: got %d, want 42", got)
	}
}

func TestEnvIntParsesNegative(t *testing.T) {
	t.Setenv("PCMI_TEST_ENVINT", "-3")
	if got := envInt("PCMI_TEST_ENVINT", 0); got != -3 {
		t.Fatalf("envInt negative: got %d, want -3", got)
	}
}

func TestEnvBoolDefaults(t *testing.T) {
	t.Setenv("PCMI_TEST_ENVBOOL", "")
	if got := envBool("PCMI_TEST_ENVBOOL", true); got != true {
		t.Fatalf("envBool default true: got %v", got)
	}
	if got := envBool("PCMI_TEST_ENVBOOL", false); got != false {
		t.Fatalf("envBool default false: got %v", got)
	}
}

func TestEnvBoolTruthyValues(t *testing.T) {
	cases := []string{"true", "1", "yes"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			t.Setenv("PCMI_TEST_ENVBOOL", v)
			if got := envBool("PCMI_TEST_ENVBOOL", false); got != true {
				t.Fatalf("envBool %q: got false, want true", v)
			}
		})
	}
}

func TestEnvBoolFalsyValues(t *testing.T) {
	cases := []string{"false", "0", "no", "FALSE", "off"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			t.Setenv("PCMI_TEST_ENVBOOL", v)
			if got := envBool("PCMI_TEST_ENVBOOL", true); got != false {
				t.Fatalf("envBool %q: got true, want false", v)
			}
		})
	}
}

func TestEnvBoolTrimsWhitespace(t *testing.T) {
	t.Setenv("PCMI_TEST_ENVBOOL", "  true  ")
	if got := envBool("PCMI_TEST_ENVBOOL", false); got != true {
		t.Fatalf("envBool whitespace: got false, want true")
	}
}

// ─── Validate edge cases ──────────────────────────────────────────────────────

func TestValidateRequireAdminAPIKey(t *testing.T) {
	cfg := &Config{
		AdminAPIKey:             "",
		DistillationBatchSize:   10,
		DistillationConcurrency: 4,
		PruneRetentionDays:      30,
		PruneIntervalSecs:       3600,
		ExpiryIntervalSecs:      3600,
		WebhookMaxAttempts:      5,
		RateLimitRPM:            60,
	}
	err := cfg.Validate(RequireAdminAPIKey)
	if err == nil {
		t.Fatal("expected ADMIN_API_KEY validation error")
	}
}

func TestValidateRequireEncryptionKey(t *testing.T) {
	cfg := &Config{
		EncryptionKey:           "",
		DistillationBatchSize:   10,
		DistillationConcurrency: 4,
		PruneRetentionDays:      30,
		PruneIntervalSecs:       3600,
		ExpiryIntervalSecs:      3600,
		WebhookMaxAttempts:      5,
		RateLimitRPM:            60,
	}
	err := cfg.Validate(RequireEncryptionKey)
	if err == nil {
		t.Fatal("expected PCMI_ENCRYPTION_KEY validation error")
	}
}

func TestPruneAndExpiryIntervalDurations(t *testing.T) {
	cfg := &Config{PruneIntervalSecs: 60, ExpiryIntervalSecs: 30}
	if got := cfg.PruneInterval().Seconds(); got != 60 {
		t.Fatalf("PruneInterval: got %v, want 60s", got)
	}
	if got := cfg.ExpiryInterval().Seconds(); got != 30 {
		t.Fatalf("ExpiryInterval: got %v, want 30s", got)
	}
}

func TestAPIAndWorkerConfigReturnSelf(t *testing.T) {
	cfg := &Config{APIPort: "8000"}
	if cfg.APIConfig() != cfg {
		t.Error("APIConfig should return the same *Config")
	}
	if cfg.WorkerConfig() != cfg {
		t.Error("WorkerConfig should return the same *Config")
	}
}
