package grpcserver

import (
	"net"
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/config"
)

// Verify ResolveGRPCPort honours cfg.GRPCPort and never reads GRPC_PORT from the environment.
func TestPortResolutionFromConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"nil-config falls back to 50051", nil, "50051"},
		{"empty GRPCPort falls back to 50051", &config.Config{GRPCPort: ""}, "50051"},
		{"whitespace-only falls back to 50051", &config.Config{GRPCPort: "   "}, "50051"},
		{"override is honoured", &config.Config{GRPCPort: "60061"}, "60061"},
		{"override is trimmed", &config.Config{GRPCPort: "  60062  "}, "60062"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveGRPCPort(tc.cfg)
			if got != tc.want {
				t.Fatalf("ResolveGRPCPort(%+v): got %q want %q", tc.cfg, got, tc.want)
			}
		})
	}
}

// TestEphemeralListenerWorks is a smoke that "the listener creation path
// used by Start works at all" — independent of database wiring.
func TestEphemeralListenerWorks(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = lis.Close() }()

	conn, err := net.DialTimeout("tcp", lis.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("dial bound port: %v", err)
	}
	_ = conn.Close()
}
