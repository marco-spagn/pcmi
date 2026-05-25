package model

import (
	"strings"
	"testing"
)

var sinkErr error

// BenchmarkValidateLtreePath_Valid benchmarks a valid 4-level path.
func BenchmarkValidateLtreePath_Valid(b *testing.B) {
	path := "root.user_42.session_7.event_99"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkErr = ValidateLtreePath(path)
	}
}

// BenchmarkValidateLtreePath_Invalid benchmarks an invalid path that triggers early exit.
func BenchmarkValidateLtreePath_Invalid(b *testing.B) {
	path := "root..invalid"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkErr = ValidateLtreePath(path)
	}
}

// BenchmarkValidateLtreePath_Long benchmarks a max-length path (256 chars).
func BenchmarkValidateLtreePath_Long(b *testing.B) {
	// Build a valid path that is exactly 256 characters: labels of 8 chars joined by dots.
	// "abcdefgh" (8) + "." = 9 chars per segment; 256/9 ≈ 28 segments, then trim to 256.
	label := "abcdefgh"
	var sb strings.Builder
	for sb.Len() < 256 {
		if sb.Len() > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(label)
	}
	path := sb.String()[:256]
	// Remove trailing dot if the truncation landed on one.
	path = strings.TrimRight(path, ".")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkErr = ValidateLtreePath(path)
	}
}
