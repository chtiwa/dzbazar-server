package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sync"
)

// Encrypts third-party API credentials (Pixel.AccessToken,
// DeliveryCompany.Token/MerchantID) at rest with AES-256-GCM. Key comes from
// FIELD_ENCRYPTION_KEY (32-byte, base64 std-encoding) — see .env.example.
// Stored format: base64(nonce || ciphertext).

var (
	fieldKeyOnce sync.Once
	fieldKey     []byte
	fieldKeyErr  error
)

func loadFieldEncryptionKey() ([]byte, error) {
	fieldKeyOnce.Do(func() {
		raw := os.Getenv("FIELD_ENCRYPTION_KEY")
		if raw == "" {
			fieldKeyErr = fmt.Errorf("FIELD_ENCRYPTION_KEY is not set")
			return
		}
		key, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			fieldKeyErr = fmt.Errorf("FIELD_ENCRYPTION_KEY is not valid base64: %w", err)
			return
		}
		if len(key) != 32 {
			fieldKeyErr = fmt.Errorf("FIELD_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
			return
		}
		fieldKey = key
	})
	return fieldKey, fieldKeyErr
}

func newFieldGCM() (cipher.AEAD, error) {
	key, err := loadFieldEncryptionKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncryptField encrypts plaintext for storage. Returns "" for "" (so empty/
// unset credential fields don't round-trip into a non-empty ciphertext).
func EncryptField(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	gcm, err := newFieldGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptField reverses EncryptField.
func DecryptField(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	gcm, err := newFieldGCM()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("invalid ciphertext encoding: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
