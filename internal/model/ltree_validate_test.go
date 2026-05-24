package model

import "testing"

func TestValidateLtreePath(t *testing.T) {
	valid := []string{
		"root",
		"root.security",
		"root.security.alerts.sql_injection",
		"root.trading.strategies.mean_reversion",
		"root.clinical.pharma.interactions",
		"A",
		"abc_123",
		"root.devops.services.payment_api",
	}
	for _, p := range valid {
		if err := ValidateLtreePath(p); err != nil {
			t.Errorf("ValidateLtreePath(%q) unexpectedly failed: %v", p, err)
		}
	}

	invalid := []struct {
		path   string
		reason string
	}{
		{"", "empty"},
		{".", "dot only"},
		{".root", "leading dot"},
		{"root.", "trailing dot"},
		{"root..security", "double dot"},
		{"root.my path", "space in label"},
		{"root.my-path", "hyphen in label"},
		{"root.héllo", "non-ASCII"},
		{"root.@alert", "special char @"},
		{string(make([]byte, 257)), "too long"},
	}
	for _, tc := range invalid {
		if err := ValidateLtreePath(tc.path); err == nil {
			t.Errorf("ValidateLtreePath(%q) expected error (%s), got nil", tc.path, tc.reason)
		}
	}
}
