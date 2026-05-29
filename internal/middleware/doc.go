// Package middleware applies API key authentication (SHA-256 hash + tenant RLS), rate limiting
// per key, read/write/admin roles, and request auditing. Health and metrics are exempted
// where security allows; see individual files for exceptions.
package middleware
