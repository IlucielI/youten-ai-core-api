package hasher_test

import (
	"testing"

	"code-base-golang/internal/pkg/hasher"
)

func TestHashPassword_Success(t *testing.T) {
	rawPassword := "SecurePassword123!"

	hash, err := hasher.HashPassword(rawPassword)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if hash == rawPassword {
		t.Fatal("hash should not equal raw password")
	}

	if !hasher.VerifyPassword(hash, rawPassword) {
		t.Fatal("expected VerifyPassword to return true for matching password")
	}
}

func TestVerifyPassword_Mismatch(t *testing.T) {
	hash, err := hasher.HashPassword("CorrectPassword123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasher.VerifyPassword(hash, "WrongPassword123") {
		t.Fatal("expected VerifyPassword to return false for non-matching password")
	}

	if hasher.VerifyPassword(hash, "") {
		t.Fatal("expected VerifyPassword to return false for empty password")
	}
}

func TestHashToken(t *testing.T) {
	rawToken := "sample-jwt-or-opaque-token"
	h1 := hasher.HashToken(rawToken)
	h2 := hasher.HashToken(rawToken)

	if h1 == "" {
		t.Fatal("expected non-empty token hash")
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %s and %s", h1, h2)
	}

	diffHash := hasher.HashToken("different-token")
	if h1 == diffHash {
		t.Fatal("expected different hashes for different inputs")
	}
}
