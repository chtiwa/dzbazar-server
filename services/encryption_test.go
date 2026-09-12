package services

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"sync"
	"testing"
)

func TestEncryptFieldRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	os.Setenv("FIELD_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
	fieldKeyOnce = sync.Once{}

	ciphertext, err := EncryptField("hello")
	if err != nil {
		t.Fatalf("EncryptField error: %v", err)
	}
	if ciphertext == "hello" {
		t.Fatalf("ciphertext must not equal plaintext")
	}

	plaintext, err := DecryptField(ciphertext)
	if err != nil {
		t.Fatalf("DecryptField error: %v", err)
	}
	if plaintext != "hello" {
		t.Fatalf("expected %q, got %q", "hello", plaintext)
	}
}
