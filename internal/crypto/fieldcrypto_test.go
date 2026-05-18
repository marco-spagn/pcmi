package crypto

import (
	"os"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv("PCMI_ENCRYPTION_KEY", "01234567890123456789012345678901")

	plain := "secret memory content"
	enc, err := EncryptContent(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain {
		t.Fatal("expected encrypted blob")
	}
	got, err := DecryptContent(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestShouldEncrypt(t *testing.T) {
	if !ShouldEncrypt(true, nil) {
		t.Fatal("encrypt_content flag")
	}
	if !ShouldEncrypt(false, map[string]interface{}{"sensitive": true}) {
		t.Fatal("metadata sensitive bool")
	}
	if !ShouldEncrypt(false, map[string]interface{}{"sensitive": "true"}) || !ShouldEncrypt(false, map[string]interface{}{"sensitive": "1"}) {
		t.Fatal("metadata sensitive string")
	}
	if ShouldEncrypt(false, map[string]interface{}{"sensitive": "false"}) {
		t.Fatal("expected false")
	}
	if ShouldEncrypt(false, map[string]interface{}{"sensitive": "True"}) != true {
		t.Fatal("EqualFold True")
	}
	if ShouldEncrypt(false, map[string]interface{}{"sensitive": "FALSE"}) {
		t.Fatal("EqualFold false")
	}
	if ShouldEncrypt(false, map[string]interface{}{"sensitive": 42}) {
		t.Fatal("unexpected type")
	}
}

func TestDecryptPlainPassthrough(t *testing.T) {
	os.Unsetenv("PCMI_ENCRYPTION_KEY")
	got, err := DecryptContent("hello")
	if err != nil || got != "hello" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestEncryptContent_missingKey(t *testing.T) {
	t.Setenv("PCMI_ENCRYPTION_KEY", "")
	_, err := EncryptContent("secret")
	if err == nil {
		t.Fatal("expected error when PCMI_ENCRYPTION_KEY unset")
	}
}

func TestEncryptContent_invalidBase64Key(t *testing.T) {
	t.Setenv("PCMI_ENCRYPTION_KEY", "@@@not-valid-base64@@@")
	_, err := EncryptContent("x")
	if err == nil {
		t.Fatal("expected error for malformed PCMI_ENCRYPTION_KEY")
	}
}

func TestEncryptContent_wrongKeyLengthAfterDecode(t *testing.T) {
	// Base64 for 1 byte — decodes but is not 32 bytes raw.
	t.Setenv("PCMI_ENCRYPTION_KEY", "YQ==")
	_, err := EncryptContent("x")
	if err == nil {
		t.Fatal("expected error when key decodes to wrong length")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	t.Setenv("PCMI_ENCRYPTION_KEY", "01234567890123456789012345678901")
	_, err := DecryptContent(encPrefix + "not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestDecryptTooShortAfterDecode(t *testing.T) {
	t.Setenv("PCMI_ENCRYPTION_KEY", "01234567890123456789012345678901")
	// Valid base64 but shorter than nonce size after decode
	_, err := DecryptContent(encPrefix + "YQ==") // single byte "a"
	if err == nil {
		t.Fatal("expected ciphertext too short")
	}
}
