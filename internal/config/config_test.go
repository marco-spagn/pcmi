package config

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Load — defaults
// ---------------------------------------------------------------------------

func TestLoad_Defaults(t *testing.T) {
	clearConfigEnv(t)

	cfg := Load()

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"RedisAddr", cfg.RedisAddr, "redis:6379"},
		{"APIPort", cfg.APIPort, "8000"},
		{"GRPCPort", cfg.GRPCPort, "50051"},
		{"EmbeddingModel", cfg.EmbeddingModel, "text-embedding-3-small"},
		{"DistillationModel", cfg.DistillationModel, "gpt-4o-mini"},
		{"DistillationBatchSize", cfg.DistillationBatchSize, 10},
		{"PruneRetentionDays", cfg.PruneRetentionDays, 30},
		{"PruneIntervalSecs", cfg.PruneIntervalSecs, 3600},
		{"ExpiryIntervalSecs", cfg.ExpiryIntervalSecs, 3600},
		{"WebhookMaxAttempts", cfg.WebhookMaxAttempts, 5},
		{"RateLimitDisabled", cfg.RateLimitDisabled, false},
		{"RateLimitRPM", cfg.RateLimitRPM, 60},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("default %s: got %v, want %v", c.name, c.got, c.want)
			}
		})
	}

	// Empty-string fields with no default
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL should be empty by default, got %q", cfg.DatabaseURL)
	}
	if cfg.OpenAIAPIKey != "" {
		t.Errorf("OpenAIAPIKey should be empty by default, got %q", cfg.OpenAIAPIKey)
	}
	if cfg.EncryptionKey != "" {
		t.Errorf("EncryptionKey should be empty by default, got %q", cfg.EncryptionKey)
	}
}

// ---------------------------------------------------------------------------
// Load — env var overrides
// ---------------------------------------------------------------------------

func TestLoad_EnvOverride(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	t.Setenv("REDIS_ADDR", "myredis:6380")
	t.Setenv("API_PORT", "9000")
	t.Setenv("GRPC_PORT", "50052")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("EMBEDDING_MODEL", "text-embedding-ada-002")
	t.Setenv("DISTILLATION_MODEL", "gpt-4o")
	t.Setenv("DISTILLATION_BATCH_SIZE", "25")
	t.Setenv("PRUNE_RETENTION_DAYS", "90")
	t.Setenv("PRUNE_INTERVAL_SECS", "7200")
	t.Setenv("EXPIRY_INTERVAL_SECS", "1800")
	t.Setenv("WEBHOOK_MAX_ATTEMPTS", "10")
	t.Setenv("RATE_LIMIT_RPM", "120")
	t.Setenv("PCMI_ENCRYPTION_KEY", "  mykey  ")

	cfg := Load()

	if cfg.DatabaseURL != "postgres://user:pass@host:5432/db" {
		t.Errorf("DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.RedisAddr != "myredis:6380" {
		t.Errorf("RedisAddr: %q", cfg.RedisAddr)
	}
	if cfg.APIPort != "9000" {
		t.Errorf("APIPort: %q", cfg.APIPort)
	}
	if cfg.GRPCPort != "50052" {
		t.Errorf("GRPCPort: %q", cfg.GRPCPort)
	}
	if cfg.OpenAIAPIKey != "sk-test" {
		t.Errorf("OpenAIAPIKey: %q", cfg.OpenAIAPIKey)
	}
	if cfg.EmbeddingModel != "text-embedding-ada-002" {
		t.Errorf("EmbeddingModel: %q", cfg.EmbeddingModel)
	}
	if cfg.DistillationModel != "gpt-4o" {
		t.Errorf("DistillationModel: %q", cfg.DistillationModel)
	}
	if cfg.DistillationBatchSize != 25 {
		t.Errorf("DistillationBatchSize: %d", cfg.DistillationBatchSize)
	}
	if cfg.PruneRetentionDays != 90 {
		t.Errorf("PruneRetentionDays: %d", cfg.PruneRetentionDays)
	}
	if cfg.PruneIntervalSecs != 7200 {
		t.Errorf("PruneIntervalSecs: %d", cfg.PruneIntervalSecs)
	}
	if cfg.ExpiryIntervalSecs != 1800 {
		t.Errorf("ExpiryIntervalSecs: %d", cfg.ExpiryIntervalSecs)
	}
	if cfg.WebhookMaxAttempts != 10 {
		t.Errorf("WebhookMaxAttempts: %d", cfg.WebhookMaxAttempts)
	}
	if cfg.RateLimitRPM != 120 {
		t.Errorf("RateLimitRPM: %d", cfg.RateLimitRPM)
	}
	// EncryptionKey trims spaces
	if cfg.EncryptionKey != "mykey" {
		t.Errorf("EncryptionKey not trimmed: %q", cfg.EncryptionKey)
	}
}

// ---------------------------------------------------------------------------
// Load — bool parsing
// ---------------------------------------------------------------------------

func TestLoad_EnvBoolVariants(t *testing.T) {
	trueCases := []string{"true", "1", "yes"}
	falseCases := []string{"false", "0", "no", ""}

	for _, v := range trueCases {
		t.Run("true:"+v, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("RATE_LIMIT_DISABLED", v)
			cfg := Load()
			if !cfg.RateLimitDisabled {
				t.Errorf("RATE_LIMIT_DISABLED=%q should parse as true", v)
			}
		})
	}

	for _, v := range falseCases {
		t.Run("false:"+v, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("RATE_LIMIT_DISABLED", v)
			cfg := Load()
			if cfg.RateLimitDisabled {
				t.Errorf("RATE_LIMIT_DISABLED=%q should parse as false", v)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Load — int with invalid value falls back to default
// ---------------------------------------------------------------------------

func TestLoad_EnvIntInvalidFallsToDefault(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DISTILLATION_BATCH_SIZE", "not-a-number")
	t.Setenv("PRUNE_RETENTION_DAYS", "abc")
	t.Setenv("WEBHOOK_MAX_ATTEMPTS", "  ")

	cfg := Load()

	if cfg.DistillationBatchSize != 10 {
		t.Errorf("DISTILLATION_BATCH_SIZE invalid: want default 10, got %d", cfg.DistillationBatchSize)
	}
	if cfg.PruneRetentionDays != 30 {
		t.Errorf("PRUNE_RETENTION_DAYS invalid: want default 30, got %d", cfg.PruneRetentionDays)
	}
	if cfg.WebhookMaxAttempts != 5 {
		t.Errorf("WEBHOOK_MAX_ATTEMPTS blank: want default 5, got %d", cfg.WebhookMaxAttempts)
	}
}

// ---------------------------------------------------------------------------
// Validate — required fields
// ---------------------------------------------------------------------------

func TestValidate_MissingDatabaseURL(t *testing.T) {
	cfg := &Config{
		RedisAddr:             "redis:6379",
		APIPort:               "8000",
		DistillationBatchSize: 10,
		PruneRetentionDays:    30,
		PruneIntervalSecs:     3600,
		ExpiryIntervalSecs:    3600,
		WebhookMaxAttempts:    5,
		RateLimitRPM:          60,
	}

	err := cfg.Validate(RequireDatabaseURL)
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Errorf("error should mention DATABASE_URL, got: %v", err)
	}
}

func TestValidate_MissingAdminAPIKey(t *testing.T) {
	cfg := minimalValidConfig()

	err := cfg.Validate(RequireAdminAPIKey)
	if err == nil {
		t.Fatal("expected error for missing ADMIN_API_KEY")
	}
	if !strings.Contains(err.Error(), "ADMIN_API_KEY is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_MissingEncryptionKey(t *testing.T) {
	cfg := minimalValidConfig()

	err := cfg.Validate(RequireEncryptionKey)
	if err == nil {
		t.Fatal("expected error for missing PCMI_ENCRYPTION_KEY")
	}
	if !strings.Contains(err.Error(), "PCMI_ENCRYPTION_KEY is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_AllRequiredFieldsPresent(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.AdminAPIKey = "admin-key"
	cfg.EncryptionKey = "enc-key"

	err := cfg.Validate(RequireDatabaseURL, RequireAdminAPIKey, RequireEncryptionKey)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Validate — range checks
// ---------------------------------------------------------------------------

func TestValidate_RangeErrors(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantMsg string
	}{
		{
			name:    "batch_size_zero",
			mutate:  func(c *Config) { c.DistillationBatchSize = 0 },
			wantMsg: "DISTILLATION_BATCH_SIZE",
		},
		{
			name:    "batch_size_over_max",
			mutate:  func(c *Config) { c.DistillationBatchSize = 1001 },
			wantMsg: "DISTILLATION_BATCH_SIZE",
		},
		{
			name:    "prune_retention_zero",
			mutate:  func(c *Config) { c.PruneRetentionDays = 0 },
			wantMsg: "PRUNE_RETENTION_DAYS",
		},
		{
			name:    "prune_interval_zero",
			mutate:  func(c *Config) { c.PruneIntervalSecs = 0 },
			wantMsg: "PRUNE_INTERVAL_SECS",
		},
		{
			name:    "expiry_interval_zero",
			mutate:  func(c *Config) { c.ExpiryIntervalSecs = 0 },
			wantMsg: "EXPIRY_INTERVAL_SECS",
		},
		{
			name:    "webhook_attempts_zero",
			mutate:  func(c *Config) { c.WebhookMaxAttempts = 0 },
			wantMsg: "WEBHOOK_MAX_ATTEMPTS",
		},
		{
			name:    "webhook_attempts_over_max",
			mutate:  func(c *Config) { c.WebhookMaxAttempts = 101 },
			wantMsg: "WEBHOOK_MAX_ATTEMPTS",
		},
		{
			name:    "rate_limit_rpm_zero",
			mutate:  func(c *Config) { c.RateLimitRPM = 0 },
			wantMsg: "RATE_LIMIT_RPM",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := minimalValidConfig()
			tc.mutate(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("expected error to mention %q, got: %v", tc.wantMsg, err)
			}
		})
	}
}

// Boundary: batch_size = 1 and 1000 are valid.
func TestValidate_BatchSizeBoundaryValid(t *testing.T) {
	for _, size := range []int{1, 1000} {
		cfg := minimalValidConfig()
		cfg.DistillationBatchSize = size

		if err := cfg.Validate(); err != nil {
			t.Errorf("batch size %d should be valid, got: %v", size, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Validate — multiple errors collected at once
// ---------------------------------------------------------------------------

func TestValidate_MultipleErrorsReported(t *testing.T) {
	cfg := &Config{
		// DatabaseURL empty → required error
		DistillationBatchSize: 0,    // range error
		PruneRetentionDays:    0,    // range error
		PruneIntervalSecs:     3600,
		ExpiryIntervalSecs:    3600,
		WebhookMaxAttempts:    5,
		RateLimitRPM:          60,
	}

	err := cfg.Validate(RequireDatabaseURL)
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()

	mustContain := []string{
		"DATABASE_URL is required",
		"DISTILLATION_BATCH_SIZE",
		"PRUNE_RETENTION_DAYS",
	}
	for _, want := range mustContain {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got:\n%s", want, msg)
		}
	}
}

// ---------------------------------------------------------------------------
// Validate — no required fields requested
// ---------------------------------------------------------------------------

func TestValidate_NoRequiredFields(t *testing.T) {
	cfg := minimalValidConfig()
	// Even without requiring DATABASE_URL, it should pass range checks
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Duration helpers
// ---------------------------------------------------------------------------

func TestPruneInterval(t *testing.T) {
	cfg := &Config{PruneIntervalSecs: 7200}
	got := cfg.PruneInterval()
	want := 2 * time.Hour
	if got != want {
		t.Errorf("PruneInterval: got %v, want %v", got, want)
	}
}

func TestExpiryInterval(t *testing.T) {
	cfg := &Config{ExpiryIntervalSecs: 1800}
	got := cfg.ExpiryInterval()
	want := 30 * time.Minute
	if got != want {
		t.Errorf("ExpiryInterval: got %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// RequiredField slices
// ---------------------------------------------------------------------------

func TestAPIRequiredFields_ContainsDatabaseURL(t *testing.T) {
	found := false
	for _, f := range APIRequiredFields {
		if f == RequireDatabaseURL {
			found = true
		}
	}
	if !found {
		t.Error("APIRequiredFields must include RequireDatabaseURL")
	}
}

func TestWorkerRequiredFields_ContainsDatabaseURL(t *testing.T) {
	found := false
	for _, f := range WorkerRequiredFields {
		if f == RequireDatabaseURL {
			found = true
		}
	}
	if !found {
		t.Error("WorkerRequiredFields must include RequireDatabaseURL")
	}
}

// ---------------------------------------------------------------------------
// APIConfig / WorkerConfig convenience views
// ---------------------------------------------------------------------------

func TestAPIConfig_ReturnsSamePointer(t *testing.T) {
	cfg := minimalValidConfig()
	if cfg.APIConfig() != cfg {
		t.Error("APIConfig() should return the same *Config pointer")
	}
}

func TestWorkerConfig_ReturnsSamePointer(t *testing.T) {
	cfg := minimalValidConfig()
	if cfg.WorkerConfig() != cfg {
		t.Error("WorkerConfig() should return the same *Config pointer")
	}
}

// ---------------------------------------------------------------------------
// OTEL fields — trim whitespace
// ---------------------------------------------------------------------------

func TestLoad_OTELFieldsTrimmed(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "  http://jaeger:4317  ")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "  http://otel:4318  ")
	t.Setenv("OTEL_SERVICE_NAME", "  pcmi-api  ")

	cfg := Load()

	if cfg.OTELTracesEndpoint != "http://jaeger:4317" {
		t.Errorf("OTELTracesEndpoint not trimmed: %q", cfg.OTELTracesEndpoint)
	}
	if cfg.OTELEndpoint != "http://otel:4318" {
		t.Errorf("OTELEndpoint not trimmed: %q", cfg.OTELEndpoint)
	}
	if cfg.OTELServiceName != "pcmi-api" {
		t.Errorf("OTELServiceName not trimmed: %q", cfg.OTELServiceName)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// minimalValidConfig returns a *Config that passes Validate() with no required fields.
func minimalValidConfig() *Config {
	return &Config{
		DatabaseURL:           "postgres://pcmi:pcmi@localhost:5432/pcmi",
		RedisAddr:             "localhost:6379",
		APIPort:               "8000",
		DistillationBatchSize: 10,
		PruneRetentionDays:    30,
		PruneIntervalSecs:     3600,
		ExpiryIntervalSecs:    3600,
		WebhookMaxAttempts:    5,
		RateLimitRPM:          60,
	}
}

// clearConfigEnv unsets all environment variables read by Load() so each test
// starts from a clean slate regardless of the host environment.
func clearConfigEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"DATABASE_URL", "DATABASE_READ_URL",
		"REDIS_ADDR", "API_PORT", "GRPC_PORT",
		"ADMIN_API_KEY", "OPENAI_API_KEY",
		"EMBEDDING_MODEL", "DISTILLATION_MODEL",
		"DISTILLATION_BATCH_SIZE", "PRUNE_RETENTION_DAYS",
		"PRUNE_INTERVAL_SECS", "EXPIRY_INTERVAL_SECS",
		"WEBHOOK_MAX_ATTEMPTS",
		"RATE_LIMIT_DISABLED", "RATE_LIMIT_RPM",
		"RATE_LIMIT_RPM_ADMIN", "RATE_LIMIT_RPM_WRITE", "RATE_LIMIT_RPM_READONLY",
		"PCMI_ENCRYPTION_KEY", "PCMI_TLS_CERT", "PCMI_TLS_KEY",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_SERVICE_NAME",
	}
	for _, v := range vars {
		t.Setenv(v, "")
	}
}
