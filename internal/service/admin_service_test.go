package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/marco-spagn/pcmi/internal/model"
)

// ─── generateAPIKey ───────────────────────────────────────────────────────────

func TestGenerateAPIKeyFormat(t *testing.T) {
	raw, hash, err := generateAPIKey()
	if err != nil {
		t.Fatalf("generateAPIKey error: %v", err)
	}
	if !strings.HasPrefix(raw, "pcmi_") {
		t.Fatalf("raw key must start with pcmi_, got %q", raw)
	}
	// SHA-256 hex = 64 chars
	if len(hash) != 64 {
		t.Fatalf("expected 64-char hex hash, got len=%d", len(hash))
	}
}

func TestGenerateAPIKeyUnique(t *testing.T) {
	r1, h1, _ := generateAPIKey()
	r2, h2, _ := generateAPIKey()
	if r1 == r2 {
		t.Fatal("two calls to generateAPIKey must return different raw keys")
	}
	if h1 == h2 {
		t.Fatal("two calls to generateAPIKey must return different hashes")
	}
}

func TestGenerateAPIKeyHashMatchesSHA256(t *testing.T) {
	raw, hash, err := generateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(raw))
	want := hex.EncodeToString(h[:])
	if hash != want {
		t.Fatalf("hash mismatch: got %s want %s", hash, want)
	}
}

// ─── AdminService.CreateTenant validation ────────────────────────────────────

func TestAdminServiceCreateTenantEmptySlug(t *testing.T) {
	svc := NewAdminService(nil)
	_, err := svc.CreateTenant(t.Context(), &model.TenantCreateRequest{Slug: "", Name: "Test"})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required-fields error, got %v", err)
	}
}

func TestAdminServiceCreateTenantEmptyName(t *testing.T) {
	svc := NewAdminService(nil)
	_, err := svc.CreateTenant(t.Context(), &model.TenantCreateRequest{Slug: "acme", Name: "  "})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required-fields error, got %v", err)
	}
}

func TestAdminServiceCreateTenantBothEmpty(t *testing.T) {
	svc := NewAdminService(nil)
	_, err := svc.CreateTenant(t.Context(), &model.TenantCreateRequest{})
	if err == nil {
		t.Fatal("expected error for empty slug and name")
	}
}
