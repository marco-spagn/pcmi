package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/marco-spagn/pcmi/internal/config"
)

// OIDCVerifier validates `Authorization: Bearer <jwt>` tokens against an OIDC
// provider and maps their claims onto PCMI's (tenant, role) model. It is
// vendor-neutral: the issuer's discovery document + JWKS drive verification, so
// Keycloak, Auth0, Entra, Okta, Google, etc. all work with the same code.
//
// Authentication stays additive — a request with no bearer token falls through
// to X-API-Key auth. OIDC is only constructed when OIDC_ISSUER is set.
type OIDCVerifier struct {
	verifier    *oidc.IDTokenVerifier
	roleClaim   string
	tenantClaim string
	adminRole   string
	writeRole   string
	roRole      string
}

// oidcProvider is the subset of *oidc.Provider used here (swappable in tests).
type oidcProvider interface {
	Verifier(config *oidc.Config) *oidc.IDTokenVerifier
}

// NewOIDCVerifier builds a verifier from config by fetching the issuer's OIDC
// discovery document. Returns (nil, nil) when OIDC is disabled so callers can
// wire it unconditionally.
func NewOIDCVerifier(ctx context.Context, cfg *config.Config) (*OIDCVerifier, error) {
	if !cfg.OIDCEnabled() {
		return nil, nil
	}
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for issuer %q: %w", cfg.OIDCIssuer, err)
	}
	return newOIDCVerifierFromProvider(provider, cfg), nil
}

func newOIDCVerifierFromProvider(provider oidcProvider, cfg *config.Config) *OIDCVerifier {
	oc := &oidc.Config{ClientID: cfg.OIDCAudience}
	if cfg.OIDCAudience == "" {
		// No audience configured: skip the aud check but keep signature/iss/exp.
		oc.SkipClientIDCheck = true
	}
	return &OIDCVerifier{
		verifier:    provider.Verifier(oc),
		roleClaim:   orDefault(cfg.OIDCRoleClaim, "roles"),
		tenantClaim: orDefault(cfg.OIDCTenantClaim, "tenant_id"),
		adminRole:   orDefault(cfg.OIDCAdminRole, "pcmi-admin"),
		writeRole:   orDefault(cfg.OIDCWriteRole, "pcmi-user"),
		roRole:      orDefault(cfg.OIDCReadonlyRole, "pcmi-readonly"),
	}
}

// Authenticate verifies a raw bearer token and returns the resolved PCMI tenant
// UUID and role. Errors are intentionally generic to avoid leaking token detail.
func (v *OIDCVerifier) Authenticate(ctx context.Context, rawToken string) (tenantID, role string, err error) {
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return "", "", fmt.Errorf("token verification failed: %w", err)
	}

	var claims map[string]json.RawMessage
	if err := idToken.Claims(&claims); err != nil {
		return "", "", fmt.Errorf("cannot parse token claims: %w", err)
	}

	tenantID = strings.TrimSpace(extractString(claims[v.tenantClaim]))
	if tenantID == "" {
		return "", "", fmt.Errorf("token missing tenant claim %q", v.tenantClaim)
	}

	role = v.mapRole(claims[v.roleClaim])
	if role == "" {
		return "", "", fmt.Errorf("token roles do not map to any PCMI role")
	}
	return tenantID, role, nil
}

// mapRole resolves the IdP role claim (string or array of strings) to a PCMI
// role, honoring precedence admin > user > readonly when several match.
func (v *OIDCVerifier) mapRole(raw json.RawMessage) string {
	roles := extractStringSlice(raw)
	has := func(target string) bool {
		for _, r := range roles {
			if strings.EqualFold(strings.TrimSpace(r), target) {
				return true
			}
		}
		return false
	}
	switch {
	case has(v.adminRole):
		return "admin"
	case has(v.writeRole):
		return "user"
	case has(v.roRole):
		return "readonly"
	default:
		return ""
	}
}

// extractString reads a JSON value that may be a plain string.
func extractString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// extractStringSlice reads a JSON value that may be a string or an array of
// strings (the two shapes IdPs use for role claims).
func extractStringSlice(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	if s := extractString(raw); s != "" {
		return []string{s}
	}
	return nil
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
