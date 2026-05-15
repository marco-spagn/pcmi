package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const encPrefix = "enc:v1:"

// EncryptContent encrypts plaintext with AES-256-GCM using PCMI_ENCRYPTION_KEY (32-byte raw or base64).
func EncryptContent(plaintext string) (string, error) {
	key, err := loadKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptContent decrypts values produced by EncryptContent.
func DecryptContent(ciphertext string) (string, error) {
	if !strings.HasPrefix(ciphertext, encPrefix) {
		return ciphertext, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	key, err := loadKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

func ShouldEncrypt(reqEncrypt bool, metadata map[string]interface{}) bool {
	if reqEncrypt {
		return true
	}
	if metadata == nil {
		return false
	}
	v, ok := metadata["sensitive"]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	default:
		return false
	}
}

func loadKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("PCMI_ENCRYPTION_KEY"))
	if raw == "" {
		return nil, errors.New("PCMI_ENCRYPTION_KEY is not set")
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("PCMI_ENCRYPTION_KEY must be 32 raw bytes or base64: %w", err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("PCMI_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(decoded))
	}
	return decoded, nil
}
