package service

import "unicode/utf8"

// truncateUTF8 caps s at maxBytes bytes without splitting a multi-byte UTF-8
// rune, so the result is always valid UTF-8. A naive s[:maxBytes] can land in
// the middle of a rune and emit an invalid byte sequence (garbled output once
// JSON-encoded, or a rejected write if persisted). The partial trailing rune,
// if any, is dropped.
func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	b := maxBytes
	for b > 0 && !utf8.RuneStart(s[b]) {
		b--
	}
	return s[:b]
}
