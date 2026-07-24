package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/marco-spagn/pcmi/internal/config"
)

const testTenant = "00000000-0000-0000-0000-000000000000"

// testIDP is a minimal in-process OIDC provider: it serves a discovery
// document + JWKS and signs tokens with an ephemeral RSA key, so the verifier
// exercises the real go-oidc discovery/JWKS/signature path.
type testIDP struct {
	server *httptest.Server
	signer jose.Signer
	issuer string
}

func newTestIDP(t *testing.T) *testIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const kid = "test-key-1"
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", kid),
	)
	if err != nil {
		t.Fatal(err)
	}
	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &key.PublicKey, KeyID: kid, Algorithm: "RS256", Use: "sig",
	}}}

	idp := &testIDP{signer: signer}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.issuer,
			"jwks_uri":                              idp.issuer + "/jwks",
			"authorization_endpoint":                idp.issuer + "/auth",
			"token_endpoint":                        idp.issuer + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	})
	idp.server = httptest.NewServer(mux)
	idp.issuer = idp.server.URL
	t.Cleanup(idp.server.Close)
	return idp
}

func (idp *testIDP) token(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := idp.signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := jws.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (idp *testIDP) verifier(t *testing.T, audience string) *OIDCVerifier {
	t.Helper()
	v, err := NewOIDCVerifier(context.Background(), &config.Config{
		OIDCIssuer:       idp.issuer,
		OIDCAudience:     audience,
		OIDCRoleClaim:    "roles",
		OIDCTenantClaim:  "tenant_id",
		OIDCAdminRole:    "pcmi-admin",
		OIDCWriteRole:    "pcmi-user",
		OIDCReadonlyRole: "pcmi-readonly",
	})
	if err != nil {
		t.Fatalf("NewOIDCVerifier: %v", err)
	}
	if v == nil {
		t.Fatal("verifier is nil (OIDC should be enabled)")
	}
	return v
}

func baseClaims(idp *testIDP, aud string, roles any) map[string]any {
	return map[string]any{
		"iss":       idp.issuer,
		"aud":       aud,
		"sub":       "user-123",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
		"tenant_id": testTenant,
		"roles":     roles,
	}
}

func TestOIDC_Disabled_ReturnsNil(t *testing.T) {
	v, err := NewOIDCVerifier(context.Background(), &config.Config{OIDCIssuer: ""})
	if err != nil || v != nil {
		t.Fatalf("disabled OIDC should return (nil, nil); got (%v, %v)", v, err)
	}
}

func TestOIDC_ValidToken_MapsRoleAndTenant(t *testing.T) {
	idp := newTestIDP(t)
	v := idp.verifier(t, "pcmi-api")

	cases := []struct {
		name  string
		roles any
		want  string
	}{
		{"user array", []string{"pcmi-user"}, "user"},
		{"admin array", []string{"pcmi-admin"}, "admin"},
		{"readonly array", []string{"pcmi-readonly"}, "readonly"},
		{"string role", "pcmi-admin", "admin"},
		{"precedence admin over readonly", []string{"pcmi-readonly", "pcmi-admin"}, "admin"},
		{"case-insensitive", []string{"PCMI-Admin"}, "admin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := idp.token(t, baseClaims(idp, "pcmi-api", tc.roles))
			tenant, role, err := v.Authenticate(context.Background(), tok)
			if err != nil {
				t.Fatalf("Authenticate: %v", err)
			}
			if tenant != testTenant {
				t.Fatalf("tenant: want %q, got %q", testTenant, tenant)
			}
			if role != tc.want {
				t.Fatalf("role: want %q, got %q", tc.want, role)
			}
		})
	}
}

func TestOIDC_Rejects(t *testing.T) {
	idp := newTestIDP(t)
	v := idp.verifier(t, "pcmi-api")

	t.Run("expired", func(t *testing.T) {
		c := baseClaims(idp, "pcmi-api", []string{"pcmi-user"})
		c["exp"] = time.Now().Add(-time.Hour).Unix()
		if _, _, err := v.Authenticate(context.Background(), idp.token(t, c)); err == nil {
			t.Fatal("expired token should be rejected")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		tok := idp.token(t, baseClaims(idp, "some-other-api", []string{"pcmi-user"}))
		if _, _, err := v.Authenticate(context.Background(), tok); err == nil {
			t.Fatal("wrong audience should be rejected")
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		c := baseClaims(idp, "pcmi-api", []string{"pcmi-user"})
		c["iss"] = "https://evil.example.com"
		if _, _, err := v.Authenticate(context.Background(), idp.token(t, c)); err == nil {
			t.Fatal("wrong issuer should be rejected")
		}
	})

	t.Run("tampered signature", func(t *testing.T) {
		tok := idp.token(t, baseClaims(idp, "pcmi-api", []string{"pcmi-user"}))
		tampered := tok[:len(tok)-3] + "AAA"
		if _, _, err := v.Authenticate(context.Background(), tampered); err == nil {
			t.Fatal("tampered token should be rejected")
		}
	})

	t.Run("missing tenant claim", func(t *testing.T) {
		c := baseClaims(idp, "pcmi-api", []string{"pcmi-user"})
		delete(c, "tenant_id")
		if _, _, err := v.Authenticate(context.Background(), idp.token(t, c)); err == nil {
			t.Fatal("missing tenant claim should be rejected")
		}
	})

	t.Run("unmapped role", func(t *testing.T) {
		tok := idp.token(t, baseClaims(idp, "pcmi-api", []string{"some-unknown-role"}))
		if _, _, err := v.Authenticate(context.Background(), tok); err == nil {
			t.Fatal("unmapped role should be rejected")
		}
	})
}

// With no audience configured the aud check is skipped, but signature/iss/exp
// are still enforced.
func TestOIDC_NoAudience_SkipsAudCheck(t *testing.T) {
	idp := newTestIDP(t)
	v := idp.verifier(t, "")
	tok := idp.token(t, baseClaims(idp, "anything-goes", []string{"pcmi-user"}))
	if _, _, err := v.Authenticate(context.Background(), tok); err != nil {
		t.Fatalf("no-audience verifier should accept any aud: %v", err)
	}
}

func TestBearerToken(t *testing.T) {
	// bearerToken is exercised via the fiber context in integration; here we
	// assert the parsing helpers used by role mapping.
	if got := extractString(json.RawMessage(`"hello"`)); got != "hello" {
		t.Fatalf("extractString: %q", got)
	}
	if got := extractStringSlice(json.RawMessage(`["a","b"]`)); len(got) != 2 {
		t.Fatalf("extractStringSlice array: %v", got)
	}
	if got := extractStringSlice(json.RawMessage(`"solo"`)); len(got) != 1 || got[0] != "solo" {
		t.Fatalf("extractStringSlice string: %v", got)
	}
}
