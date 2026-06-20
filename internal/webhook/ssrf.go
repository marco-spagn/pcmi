package webhook

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// allowPrivate, when true, disables SSRF egress filtering so webhook deliveries
// may target private/internal addresses (loopback, RFC1918, link-local incl.
// cloud metadata, ULA, …). The secure default is false: such targets are
// blocked both at registration and at dial time.
var allowPrivate atomic.Bool

// SetAllowPrivateTargets configures whether webhook deliveries may reach
// private/internal addresses. Call once at startup from config
// (WEBHOOK_ALLOW_PRIVATE_TARGETS). Not calling it leaves the secure default
// (block) in place.
func SetAllowPrivateTargets(v bool) { allowPrivate.Store(v) }

func privateTargetsAllowed() bool { return allowPrivate.Load() }

// ValidateTargetURL rejects webhook URLs that are malformed, not http(s), or —
// unless private targets are allowed — point at a literal private/internal IP.
// It runs at registration for fast, network-free feedback and deliberately does
// NOT resolve hostnames: a host that is currently unresolvable (placeholder,
// receiver temporarily down) is still accepted, and any host that resolves to a
// blocked address — including via DNS rebinding — is refused at dial time by
// GuardedHTTPClient, which is the actual enforcement boundary.
func ValidateTargetURL(raw string) error {
	return validateTargetURL(raw, privateTargetsAllowed())
}

func validateTargetURL(raw string, allow bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook url must use http or https scheme")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook url must include a host")
	}
	if allow {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && blockedIP(ip) {
		return fmt.Errorf("webhook url targets a blocked address (%s)", ip)
	}
	return nil
}

// blockedIP reports whether ip must never be a webhook target: loopback,
// private (RFC1918 / ULA fc00::/7), link-local (incl. cloud metadata
// 169.254.169.254 and fe80::/10), multicast, or the unspecified address.
func blockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// GuardedHTTPClient returns an HTTP client whose dialer refuses to connect to
// blocked addresses (checked post-resolution, so DNS rebinding and redirects to
// internal hosts are also caught) and that caps redirects. When private targets
// are allowed the guard is a no-op.
func GuardedHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if privateTargetsAllowed() {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || blockedIP(ip) {
				return fmt.Errorf("blocked webhook target address %q", address)
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:       dialer.DialContext,
			ForceAttemptHTTP2: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			if !privateTargetsAllowed() {
				if ip := net.ParseIP(req.URL.Hostname()); ip != nil && blockedIP(ip) {
					return fmt.Errorf("redirect to blocked address %s", ip)
				}
			}
			return nil
		},
	}
}
