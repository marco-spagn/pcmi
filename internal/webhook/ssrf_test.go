package webhook

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestBlockedIP(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},      // loopback
		{"10.1.2.3", true},       // RFC1918
		{"172.16.5.4", true},     // RFC1918
		{"192.168.0.1", true},    // RFC1918
		{"169.254.169.254", true}, // link-local / cloud metadata
		{"0.0.0.0", true},        // unspecified
		{"224.0.0.1", true},      // multicast
		{"::1", true},            // IPv6 loopback
		{"fe80::1", true},        // IPv6 link-local
		{"fc00::1", true},        // IPv6 unique-local
		{"::", true},             // IPv6 unspecified
		{"8.8.8.8", false},       // public
		{"1.1.1.1", false},       // public
		{"93.184.216.34", false}, // public (example.com)
		{"2606:2800:220:1:248:1893:25c8:1946", false}, // public IPv6
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := blockedIP(ip); got != c.blocked {
			t.Errorf("blockedIP(%s) = %v, want %v", c.ip, got, c.blocked)
		}
	}
}

func stubLookup(table map[string][]string) ipLookup {
	return func(_ context.Context, host string) ([]net.IPAddr, error) {
		ips, ok := table[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		out := make([]net.IPAddr, 0, len(ips))
		for _, s := range ips {
			out = append(out, net.IPAddr{IP: net.ParseIP(s)})
		}
		return out, nil
	}
}

func TestValidateTargetURL(t *testing.T) {
	t.Parallel()
	lookup := stubLookup(map[string][]string{
		"safe.example.com":     {"93.184.216.34"},
		"internal.example.com": {"10.0.0.5"},
		"mixed.example.com":    {"8.8.8.8", "127.0.0.1"}, // one bad → blocked
	})

	cases := []struct {
		name    string
		raw     string
		allow   bool
		wantErr string // substring; "" means no error
	}{
		{"public host ok", "https://safe.example.com/hook", false, ""},
		{"literal public ok", "https://8.8.8.8/hook", false, ""},
		{"ftp scheme rejected", "ftp://safe.example.com", false, "scheme"},
		{"no scheme rejected", "safe.example.com/hook", false, "scheme"},
		{"empty host rejected", "http:///hook", false, "host"},
		{"loopback literal blocked", "http://127.0.0.1:9000/x", false, "blocked"},
		{"metadata literal blocked", "http://169.254.169.254/latest/meta-data/", false, "blocked"},
		{"private resolve blocked", "https://internal.example.com/hook", false, "blocked"},
		{"mixed resolve blocked", "https://mixed.example.com/hook", false, "blocked"},
		{"unresolvable rejected", "https://nope.example.com/hook", false, "resolve"},
		{"allowPrivate bypasses", "http://127.0.0.1:9000/x", true, ""},
		{"allowPrivate still needs scheme", "ftp://127.0.0.1", true, "scheme"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateTargetURL(c.raw, c.allow, lookup)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("got %v, want error containing %q", err, c.wantErr)
			}
		})
	}
}

func TestGuardedHTTPClient_blocksLoopbackDial(t *testing.T) {
	// Not parallel: mutates the package-global allow-private flag.
	SetAllowPrivateTargets(false)
	t.Cleanup(func() { SetAllowPrivateTargets(false) })

	client := GuardedHTTPClient(2_000_000_000) // 2s
	// Dialing a loopback target must be refused by the dialer Control hook.
	_, err := client.Get("http://127.0.0.1:9/") // discard port; never connects
	if err == nil {
		t.Fatal("expected dial to be blocked, got nil error")
	}
	if !strings.Contains(err.Error(), "blocked webhook target address") {
		t.Fatalf("expected block error, got: %v", err)
	}
}
