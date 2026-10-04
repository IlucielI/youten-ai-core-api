package hasher

import (
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// DefaultBcryptCost specifies the computational workload cost for bcrypt password hashing.
const DefaultBcryptCost = 12

// HashPassword hashes a raw password using bcrypt with cost 12.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// VerifyPassword compares a bcrypt hashed password with its possible plaintext equivalent.
func VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// HashToken computes a SHA-256 hex digest of a token string for safe database storage.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
