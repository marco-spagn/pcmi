package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config centralizza tutte le variabili d'ambiente del sistema PCMI.
// Usare Load() per caricare e MustLoad() per fail-fast all'avvio.
type Config struct {
	// Database
	DatabaseURL     string
	DatabaseReadURL string // opzionale: replica di lettura

	// Redis
	RedisAddr     string
	EventBackend  string // streams (default) or pubsub — see EVENT_BACKEND

	// API Server
	APIPort string

	// gRPC
	GRPCPort string

	// Authentication
	AdminAPIKey string

	// OpenAI / Embedding
	OpenAIAPIKey   string
	OpenAIBaseURL  string // optional: proxy or Azure OpenAI endpoint base
	EmbeddingModel string

	// Distillation / Worker
	DistillationModel       string
	DistillationBatchSize   int
	DistillationConcurrency int // max parallel LLM jobs, default 4
	PruneRetentionDays      int
	PruneIntervalSecs    int
	ExpiryIntervalSecs   int
	WebhookMaxAttempts   int

	// Rate limiting
	RateLimitDisabled   bool
	RateLimitRPM        int
	RateLimitRPMAdmin   int
	RateLimitRPMWrite   int
	RateLimitRPMReadonly int

	// TLS (optional — leave empty for plain HTTP)
	TLSCertFile string
	TLSKeyFile  string

	// Encryption
	EncryptionKey string

	// OpenTelemetry
	OTELTracesEndpoint string
	OTELEndpoint       string
	OTELServiceName    string
}

// APIConfig returns the subset of fields required by the API service.
// Validation is still performed on the full Config; this is a convenience view.
func (c *Config) APIConfig() *Config { return c }

// WorkerConfig returns the subset of fields required by the Worker service.
func (c *Config) WorkerConfig() *Config { return c }

// Load reads all environment variables and applies defaults.
// It does NOT return an error — call Validate() or use MustLoad().
func Load() *Config {
	cfg := &Config{
		DatabaseURL:     envOr("DATABASE_URL", ""),
		DatabaseReadURL: os.Getenv("DATABASE_READ_URL"),

		RedisAddr:    envOr("REDIS_ADDR", "redis:6379"),
		EventBackend: envOr("EVENT_BACKEND", "streams"),
		APIPort:   envOr("API_PORT", "8000"),
		GRPCPort:  envOr("GRPC_PORT", "50051"),

		AdminAPIKey: os.Getenv("ADMIN_API_KEY"),
		OpenAIAPIKey:  os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL: strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		EmbeddingModel:   envOr("EMBEDDING_MODEL", "text-embedding-3-small"),
		DistillationModel: envOr("DISTILLATION_MODEL", "gpt-4o-mini"),

		DistillationBatchSize:   envInt("DISTILLATION_BATCH_SIZE", 10),
		DistillationConcurrency: envInt("DISTILLATION_CONCURRENCY", 4),
		PruneRetentionDays:   envInt("PRUNE_RETENTION_DAYS", 30),
		PruneIntervalSecs:    envInt("PRUNE_INTERVAL_SECS", 3600),
		ExpiryIntervalSecs:   envInt("EXPIRY_INTERVAL_SECS", 3600),
		WebhookMaxAttempts:   envInt("WEBHOOK_MAX_ATTEMPTS", 5),

		RateLimitDisabled:    envBool("RATE_LIMIT_DISABLED", false),
		RateLimitRPM:         envInt("RATE_LIMIT_RPM", 120),
		RateLimitRPMAdmin:    envInt("RATE_LIMIT_RPM_ADMIN", 30),
		RateLimitRPMWrite:    envInt("RATE_LIMIT_RPM_WRITE", 100),
		RateLimitRPMReadonly: envInt("RATE_LIMIT_RPM_READONLY", 200),

		TLSCertFile: strings.TrimSpace(os.Getenv("PCMI_TLS_CERT")),
		TLSKeyFile:  strings.TrimSpace(os.Getenv("PCMI_TLS_KEY")),

		EncryptionKey: strings.TrimSpace(os.Getenv("PCMI_ENCRYPTION_KEY")),

		OTELTracesEndpoint: strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")),
		OTELEndpoint:       strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		OTELServiceName:    strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")),
	}
	return cfg
}

// Validate verifies that all required fields are set and values are in valid ranges.
// Returns a multi-error with all problems found so the operator can fix everything at once.
func (c *Config) Validate(requiredFields ...RequiredField) error {
	var errs []string

	for _, f := range requiredFields {
		switch f {
		case RequireDatabaseURL:
			if c.DatabaseURL == "" {
				errs = append(errs, "DATABASE_URL is required")
			}
		case RequireAdminAPIKey:
			if c.AdminAPIKey == "" {
				errs = append(errs, "ADMIN_API_KEY is required")
			}
		case RequireEncryptionKey:
			if c.EncryptionKey == "" {
				errs = append(errs, "PCMI_ENCRYPTION_KEY is required")
			}
		}
	}

	// Range validations (always checked)
	if c.DistillationBatchSize < 1 || c.DistillationBatchSize > 1000 {
		errs = append(errs, fmt.Sprintf("DISTILLATION_BATCH_SIZE must be 1–1000 (got %d)", c.DistillationBatchSize))
	}
	if c.DistillationConcurrency < 1 || c.DistillationConcurrency > 16 {
		errs = append(errs, fmt.Sprintf("DISTILLATION_CONCURRENCY must be 1–16 (got %d)", c.DistillationConcurrency))
	}
	if c.PruneRetentionDays < 1 {
		errs = append(errs, fmt.Sprintf("PRUNE_RETENTION_DAYS must be ≥ 1 (got %d)", c.PruneRetentionDays))
	}
	if c.PruneIntervalSecs < 1 {
		errs = append(errs, fmt.Sprintf("PRUNE_INTERVAL_SECS must be ≥ 1 (got %d)", c.PruneIntervalSecs))
	}
	if c.ExpiryIntervalSecs < 1 {
		errs = append(errs, fmt.Sprintf("EXPIRY_INTERVAL_SECS must be ≥ 1 (got %d)", c.ExpiryIntervalSecs))
	}
	if c.WebhookMaxAttempts < 1 || c.WebhookMaxAttempts > 100 {
		errs = append(errs, fmt.Sprintf("WEBHOOK_MAX_ATTEMPTS must be 1–100 (got %d)", c.WebhookMaxAttempts))
	}
	if c.RateLimitRPM < 1 {
		errs = append(errs, fmt.Sprintf("RATE_LIMIT_RPM must be ≥ 1 (got %d)", c.RateLimitRPM))
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.New("config validation failed:\n  - " + strings.Join(errs, "\n  - "))
}

// RequiredField enumerates fields that are mandatory for a given service.
type RequiredField int

const (
	RequireDatabaseURL  RequiredField = iota
	RequireAdminAPIKey
	RequireEncryptionKey
)

// APIRequiredFields are the fields that the API service must have at startup.
var APIRequiredFields = []RequiredField{
	RequireDatabaseURL,
}

// WorkerRequiredFields are the fields that the Worker service must have at startup.
var WorkerRequiredFields = []RequiredField{
	RequireDatabaseURL,
}

// PruneInterval returns PruneIntervalSecs as a time.Duration.
func (c *Config) PruneInterval() time.Duration {
	return time.Duration(c.PruneIntervalSecs) * time.Second
}

// ExpiryInterval returns ExpiryIntervalSecs as a time.Duration.
func (c *Config) ExpiryInterval() time.Duration {
	return time.Duration(c.ExpiryIntervalSecs) * time.Second
}

// -------------------------------------------------------------------
// helpers

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1" || v == "yes"
}
