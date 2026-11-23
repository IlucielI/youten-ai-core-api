package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecrypt_Success(t *testing.T) {
	secret := "my-super-secure-app-secret-12345"
	plaintext := "ya29.a0AfH6SMD_refresh_token_example_1234567890"

	encrypted, err := Encrypt(plaintext, secret)
	if err != nil {
		t.Fatalf("unexpected encryption error: %v", err)
	}

	if encrypted == "" || encrypted == plaintext {
		t.Fatalf("expected non-empty ciphertext different from plaintext, got %q", encrypted)
	}

	decrypted, err := Decrypt(encrypted, secret)
	if err != nil {
		t.Fatalf("unexpected decryption error: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("expected decrypted %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptDecrypt_EmptySecret(t *testing.T) {
	_, err := Encrypt("test", "")
	if err == nil || !strings.Contains(err.Error(), "key cannot be empty") {
		t.Errorf("expected ErrEmptyKey, got %v", err)
	}

	_, err = Decrypt("test", "")
	if err == nil || !strings.Contains(err.Error(), "key cannot be empty") {
		t.Errorf("expected ErrEmptyKey, got %v", err)
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	secret := "secret-key-1"
	encrypted, err := Encrypt("sensitive-token", secret)
	if err != nil {
		t.Fatalf("unexpected encryption error: %v", err)
	}

	// Wrong key
	_, err = Decrypt(encrypted, "wrong-secret-key-2")
	if err == nil {
		t.Error("expected decryption failure with wrong secret, got nil")
	}

	// Truncated/corrupted ciphertext
	_, err = Decrypt("short", secret)
	if err == nil {
		t.Error("expected error on truncated ciphertext, got nil")
	}

	// Invalid base64
	_, err = Decrypt("not-valid-base64!@#$%", secret)
	if err == nil {
		t.Error("expected error on invalid base64, got nil")
	}
}

