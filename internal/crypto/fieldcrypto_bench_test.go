package crypto

import (
	"strings"
	"testing"
)

func initBenchKey(b *testing.B) {
	b.Helper()
	b.Cleanup(ResetKey)
	if err := InitKey("01234567890123456789012345678901"); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkEncryptField benchmarks encryption of a 1KB plaintext string.
func BenchmarkEncryptField(b *testing.B) {
	initBenchKey(b)
	plaintext := strings.Repeat("a", 1024)
	var (
		result string
		err    error
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err = EncryptContent(plaintext)
		if err != nil {
			b.Fatal(err)
		}
	}
	_ = result
}

// BenchmarkDecryptField benchmarks decryption of a 1KB ciphertext.
func BenchmarkDecryptField(b *testing.B) {
	initBenchKey(b)
	plaintext := strings.Repeat("a", 1024)
	ciphertext, err := EncryptContent(plaintext)
	if err != nil {
		b.Fatal(err)
	}
	var result string
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err = DecryptContent(ciphertext)
		if err != nil {
			b.Fatal(err)
		}
	}
	_ = result
}

// BenchmarkEncryptDecryptRoundTrip benchmarks a full encrypt-then-decrypt round trip on 1KB.
func BenchmarkEncryptDecryptRoundTrip(b *testing.B) {
	initBenchKey(b)
	plaintext := strings.Repeat("a", 1024)
	var (
		enc string
		dec string
		err error
	)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err = EncryptContent(plaintext)
		if err != nil {
			b.Fatal(err)
		}
		dec, err = DecryptContent(enc)
		if err != nil {
			b.Fatal(err)
		}
	}
	_ = dec
}
