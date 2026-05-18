package grpcserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"

	"github.com/marco-spagn/pcmi/internal/config"
)

// TestTLSHandshakeEndToEnd asserts the actual TLS path works — not just that
// BuildServerOptions returns the right number of options. We spin up a real
// grpc.Server with TLS enabled, listen on an ephemeral port, dial it with a
// TLS client that trusts the self-signed CA, and verify the connection
// transitions to READY.
//
// This is the regression test that catches "we built the option but never
// wired it into grpc.NewServer" / "the listener accepts plain TCP because
// Serve was called on the wrong listener" kinds of bugs.
func TestTLSHandshakeEndToEnd(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := &config.Config{TLSCertFile: certFile, TLSKeyFile: keyFile}
	srv := grpc.NewServer(BuildServerOptions(cfg)...)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.GracefulStop()
		select {
		case err := <-serveErr:
			// grpc.Server.Serve returns nil after GracefulStop.
			if err != nil && err != grpc.ErrServerStopped {
				t.Logf("Serve returned: %v", err)
			}
		case <-time.After(3 * time.Second):
			srv.Stop()
		}
	})

	// Build a TLS client that trusts our self-signed CA. We read the cert
	// file we just wrote and seed a CertPool with it — same pattern used by
	// k8s ingress-gateway tests.
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatal("failed to seed CertPool from self-signed cert")
	}
	clientTLS := &tls.Config{
		RootCAs:    pool,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//nolint:staticcheck // grpc.DialContext is the stable API in v1.81; WaitForReady is needed for the handshake assertion below.
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if state := conn.GetState(); state != connectivity.Ready {
		t.Fatalf("expected READY after blocking dial, got %v", state)
	}
}

// TestTLSHandshakeRejectsPlainClient is the converse of the happy-path test:
// a client that does NOT speak TLS must fail to handshake against a TLS
// server. Confirms BuildServerOptions actually enforces TLS, not just
// "advertises" it.
func TestTLSHandshakeRejectsPlainClient(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := &config.Config{TLSCertFile: certFile, TLSKeyFile: keyFile}
	srv := grpc.NewServer(BuildServerOptions(cfg)...)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	// Plain TCP dial — should accept the TCP connection but the TLS server
	// will drop the stream the moment it sees non-TLS bytes. We assert via
	// a tight read deadline: a TLS-only server doesn't speak gRPC frames
	// over plaintext, so any read either errors or times out.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("plain TCP dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 16)
	// We DON'T write anything — a non-TLS gRPC client would normally send
	// the HTTP/2 connection preface here. The TLS server will see invalid
	// TLS records and close, so the read returns EOF or a timeout.
	// Either outcome is acceptable; what we forbid is a clean read of any
	// data, which would indicate plain TCP works.
	n, readErr := conn.Read(buf)
	if readErr == nil && n > 0 {
		t.Fatalf("TLS server returned plaintext bytes (%d): %x — TLS not enforced", n, buf[:n])
	}
}
