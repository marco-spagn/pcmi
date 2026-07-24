package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/marco-spagn/pcmi/internal/crypto"
)

func TestResolveSecret_LiteralValue(t *testing.T) {
	t.Setenv("TEST_SECRET", "  hunter2  ")
	if got := resolveSecret("TEST_SECRET"); got != "hunter2" {
		t.Fatalf("literal: want trimmed %q, got %q", "hunter2", got)
	}
}

func TestResolveSecret_Unset(t *testing.T) {
	if got := resolveSecret("DEFINITELY_UNSET_SECRET_XYZ"); got != "" {
		t.Fatalf("unset: want empty, got %q", got)
	}
}

func TestResolveSecret_FileConvention(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "enc.key")
	if err := os.WriteFile(p, []byte("s3cr3t-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PCMI_ENCRYPTION_KEY_FILE", p)
	if got := resolveSecret("PCMI_ENCRYPTION_KEY"); got != "s3cr3t-from-file" {
		t.Fatalf("_FILE: want trimmed file contents, got %q", got)
	}
}

func TestResolveSecret_FileConventionTakesPrecedenceOverRaw(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s")
	if err := os.WriteFile(p, []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("X_SECRET", "from-env")
	t.Setenv("X_SECRET_FILE", p)
	if got := resolveSecret("X_SECRET"); got != "from-file" {
		t.Fatalf("_FILE should win over raw env: got %q", got)
	}
}

func TestResolveSecret_FileScheme(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tok")
	if err := os.WriteFile(p, []byte("tok-value"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("Y_SECRET", "file:"+p)
	if got := resolveSecret("Y_SECRET"); got != "tok-value" {
		t.Fatalf("file: scheme: got %q", got)
	}
}

func TestResolveSecret_EnvScheme(t *testing.T) {
	t.Setenv("SOURCE_VAR", "indirect-value")
	t.Setenv("Z_SECRET", "env:SOURCE_VAR")
	if got := resolveSecret("Z_SECRET"); got != "indirect-value" {
		t.Fatalf("env: scheme: got %q", got)
	}
}

func TestResolveSecret_EnvSchemeEmptyTarget(t *testing.T) {
	t.Setenv("W_SECRET", "env:")
	if got := resolveSecret("W_SECRET"); got != "" {
		t.Fatalf("env: with no target should be empty, got %q", got)
	}
}

func TestResolveSecret_MissingFileReturnsEmpty(t *testing.T) {
	t.Setenv("M_SECRET_FILE", "/no/such/path/at/all")
	if got := resolveSecret("M_SECRET"); got != "" {
		t.Fatalf("missing file: want empty (Validate surfaces it), got %q", got)
	}
}

// A literal DSN must pass through untouched — it does not start with a scheme.
func TestResolveSecret_DSNPassthrough(t *testing.T) {
	dsn := "postgres://pcmi:pcmi@localhost:5432/pcmi?sslmode=disable"
	t.Setenv("DATABASE_URL", dsn)
	if got := resolveSecret("DATABASE_URL"); got != dsn {
		t.Fatalf("DSN passthrough: got %q", got)
	}
}

// End-to-end through Load(): a secret file mounted for PCMI_ENCRYPTION_KEY must
// reach cfg.EncryptionKey exactly as the raw env value would, so field crypto
// works without the key ever being in the environment.
func TestLoad_EncryptionKeyFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "enc.key")
	key := "01234567890123456789012345678901" // 32 raw bytes, valid AES-256 key
	if err := os.WriteFile(p, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PCMI_ENCRYPTION_KEY_FILE", p)
	t.Setenv("PCMI_ENCRYPTION_KEY", "") // ensure the file path is what wins

	cfg := Load()
	if cfg.EncryptionKey != key {
		t.Fatalf("Load() EncryptionKey from _FILE: want %q, got %q", key, cfg.EncryptionKey)
	}
	// The resolved key must satisfy the required-secret validation.
	if err := cfg.Validate(RequireEncryptionKey); err != nil {
		t.Fatalf("Validate(RequireEncryptionKey) with file-sourced key: %v", err)
	}
}

// Full chain: a key mounted as a file drives real field encryption — file →
// config.Load() → crypto.InitKey → encrypt/decrypt round-trip.
func TestLoad_EncryptionKeyFromFile_CryptoRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "enc.key")
	if err := os.WriteFile(p, []byte("01234567890123456789012345678901\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PCMI_ENCRYPTION_KEY_FILE", p)
	t.Setenv("PCMI_ENCRYPTION_KEY", "")

	cfg := Load()
	if err := crypto.InitKey(cfg.EncryptionKey); err != nil {
		t.Fatalf("InitKey with file-sourced key: %v", err)
	}
	const plaintext = "sensitive memory content"
	ct, err := crypto.EncryptContent(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ct == plaintext {
		t.Fatal("ciphertext equals plaintext — encryption did not run")
	}
	pt, err := crypto.DecryptContent(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if pt != plaintext {
		t.Fatalf("round-trip mismatch: want %q, got %q", plaintext, pt)
	}
}
