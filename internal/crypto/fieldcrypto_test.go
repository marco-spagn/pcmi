package crypto

import (
	"testing"
)

func testKey(t *testing.T) {
	t.Helper()
	t.Cleanup(ResetKey)
	if err := InitKey("01234567890123456789012345678901"); err != nil {
		t.Fatal(err)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	testKey(t)

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
	ResetKey()
	got, err := DecryptContent("hello")
	if err != nil || got != "hello" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestEncryptContent_missingKey(t *testing.T) {
	ResetKey()
	_, err := EncryptContent("secret")
	if err == nil {
		t.Fatal("expected error when encryption key unset")
	}
}

func TestEncryptContent_invalidBase64Key(t *testing.T) {
	if err := InitKey("@@@not-valid-base64@@@"); err == nil {
		t.Fatal("expected error for malformed PCMI_ENCRYPTION_KEY")
	}
}

func TestEncryptContent_wrongKeyLengthAfterDecode(t *testing.T) {
	if err := InitKey("YQ=="); err == nil {
		t.Fatal("expected error when key decodes to wrong length")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	testKey(t)
	_, err := DecryptContent(encPrefix + "not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestDecryptTooShortAfterDecode(t *testing.T) {
	testKey(t)
	_, err := DecryptContent(encPrefix + "YQ==")
	if err == nil {
		t.Fatal("expected ciphertext too short")
	}
}
