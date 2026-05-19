package grpcserver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/marco-spagn/pcmi/internal/config"
)

// TestTLSHandshakeEndToEnd asserts the actual TLS path works — not just that
// BuildServerOptions returns the right number of options. We spin up a real
// grpc.Server with TLS enabled, listen on an ephemeral port, dial it with a
// TLS-aware grpc.NewClient (the v1.63+ replacement for the deprecated
// grpc.DialContext + grpc.WithBlock pair), and wait until the connection
// transitions to READY.
//
// This is the regression test that catches "we built the option but never
// wired it into grpc.NewServer" / "the listener accepts plain TCP because
// Serve was called on the wrong listener" bugs.
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
			// grpc.Server.Serve returns nil after GracefulStop. ErrServerStopped
			// can surface depending on the race between Serve and Stop.
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
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

	// grpc.NewClient is the v1.63+ API. It does not block; we then call
	// Connect() and poll for READY via WaitForStateChange — this drives the
	// TLS handshake to completion deterministically.
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	conn.Connect()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return
		}
		if !conn.WaitForStateChange(ctx, state) {
			t.Fatalf("connection did not reach READY (last state: %v): %v", state, ctx.Err())
		}
	}
}

// TestTLSHandshakeEndToEnd_HealthRPC exercises one RPC after TLS comes up so we
// catch regressions where creds allow handshake but ServeOptions break handlers.
func TestTLSHandshakeEndToEnd_HealthRPC(t *testing.T) {
	certFile, keyFile := writeSelfSignedCert(t)

	cfg := &config.Config{TLSCertFile: certFile, TLSKeyFile: keyFile}
	srv := grpc.NewServer(BuildServerOptions(cfg)...)
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())

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
			if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				t.Logf("Serve returned: %v", err)
			}
		case <-time.After(3 * time.Second):
			srv.Stop()
		}
	})

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

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	defer func() { _ = conn.Close() }()

	conn.Connect()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if !conn.WaitForStateChange(ctx, state) {
			t.Fatalf("connection did not reach READY (last state: %v): %v", state, ctx.Err())
		}
	}

	hc := grpc_health_v1.NewHealthClient(conn)
	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Health.Check: %v", err)
	}
	if resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected health status: %v", resp.GetStatus())
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
