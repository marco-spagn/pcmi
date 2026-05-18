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
