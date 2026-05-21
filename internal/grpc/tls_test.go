package grpcserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/marco-spagn/pcmi/internal/config"
)

// TestBuildServerOptions_NoTLSWhenUnset locks in the default behaviour:
// without TLS env vars, BuildServerOptions returns a single ServerOption
// (the OTel stats handler) and no credentials are attached.
func TestBuildServerOptions_NoTLSWhenUnset(t *testing.T) {
	opts := BuildServerOptions(&config.Config{})
	if len(opts) != 1 {
		t.Fatalf("expected 1 option (StatsHandler only), got %d", len(opts))
	}
	// Sanity: nil cfg also tolerated.
	if got := BuildServerOptions(nil); len(got) != 1 {
		t.Fatalf("nil cfg: expected 1 option, got %d", len(got))
	}
}

// TestBuildServerOptions_TLSEnabled writes a self-signed cert+key to tempdir
// and verifies BuildServerOptions returns a 2-element slice (stats handler +
// grpc.Creds(tls)). End-to-end TLS handshake is covered by
// TestTLSHandshakeEndToEnd in tls_handshake_test.go.
func TestBuildServerOptions_TLSEnabled(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := &config.Config{TLSCertFile: certFile, TLSKeyFile: keyFile}
	opts := BuildServerOptions(cfg)
	if len(opts) != 2 {
		t.Fatalf("expected 2 options (StatsHandler + Creds), got %d", len(opts))
	}
	// Construct a real server with the options — if creds were malformed,
	// grpc.NewServer would panic.
	srv := grpc.NewServer(opts...)
	srv.Stop()
}

// TestBuildServerOptions_PartialTLSFallsBack covers the "only cert" /
// "only key" misconfigurations: BuildServerOptions must log a warning and
// return plain TCP options rather than half-configuring TLS.
func TestBuildServerOptions_PartialTLSFallsBack(t *testing.T) {
	cases := []struct {
		name   string
		cfg    *config.Config
	}{
		{"only cert", &config.Config{TLSCertFile: "/tmp/nonexistent.crt"}},
		{"only key", &config.Config{TLSKeyFile: "/tmp/nonexistent.key"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := BuildServerOptions(tc.cfg)
			if len(opts) != 1 {
				t.Fatalf("expected 1 option for partial TLS, got %d", len(opts))
			}
		})
	}
}

// TestBuildServerOptions_BadCertFallsBack guarantees the server doesn't
// deadlock when the operator points TLSCertFile at a missing or malformed
// file. The function logs and falls back to plain TCP.
func TestBuildServerOptions_BadCertFallsBack(t *testing.T) {
	dir := t.TempDir()
	garbage := filepath.Join(dir, "not-a-cert.pem")
	if err := os.WriteFile(garbage, []byte("definitely not a PEM block"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{TLSCertFile: garbage, TLSKeyFile: garbage}
	opts := BuildServerOptions(cfg)
	if len(opts) != 1 {
		t.Fatalf("expected 1 option after malformed cert, got %d", len(opts))
	}
}

// TestBuildServerOptions_ReturnTypeIsServerOption is paranoia: ensure the
// slice element type matches grpc.ServerOption so grpc.NewServer accepts it
// without a compiler-side panic on future grpc/v2 bumps.
func TestBuildServerOptions_ReturnTypeIsServerOption(t *testing.T) {
	opts := BuildServerOptions(nil)
	want := reflect.TypeOf((*grpc.ServerOption)(nil)).Elem()
	got := reflect.TypeOf(opts).Elem()
	if got != want {
		t.Fatalf("expected []grpc.ServerOption, got []%v", got)
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

// writeSelfSignedCert generates a fresh ECDSA P-256 self-signed cert valid
// for 1 hour and writes both PEM files into a tempdir, returning their
// absolute paths. The cert is bound to localhost / 127.0.0.1 so it can
// double for TLS handshake tests if needed.
func writeSelfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "pcmi-test"},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "server.crt")
	keyFile = filepath.Join(dir, "server.key")

	certPEM, err := os.Create(certFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = certPEM.Close() }()
	if err := pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatal(err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.Create(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keyPEM.Close() }()
	if err := pem.Encode(keyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatal(err)
	}

	// Sanity: the file we just wrote loads as a TLS cert pair.
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("self-signed cert is unusable: %v", err)
	}
	return certFile, keyFile
}
