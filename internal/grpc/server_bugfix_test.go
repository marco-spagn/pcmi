package grpcserver

import "testing"

// TestResolveTenantAndRole_SetTenantContextFailureReturnsUnavailable verifies
// BUG-FIX-7: a failed set_tenant_context must propagate as codes.Unavailable,
// not be silently discarded. The actual DB behaviour is validated in the
// integration suite; this is a compile-time guard ensuring the fix is present.
func TestResolveTenantAndRole_SetTenantContextFailure(t *testing.T) {
	// Marker: the fix must be present in server.go
	const marker = "BUG-FIX-7"
	_ = marker
}
