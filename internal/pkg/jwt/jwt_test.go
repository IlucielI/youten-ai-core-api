package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
)

func TestGenerateTokenPair_And_Validate(t *testing.T) {
	cfg := config.Config{
		AppName:              "code-base-golang-test",
		JWTSecret:            "test-secret-key-12345",
		JWTAccessExpiration:  24 * time.Hour,
		JWTRefreshExpiration: 7 * 24 * time.Hour,
	}

	userID := uuid.New()
	email := "test@example.com"
	sessionID := uuid.New().String()

	pair, err := GenerateTokenPair(cfg, userID, email, sessionID)
	if err != nil {
		t.Fatalf("unexpected error generating token pair: %v", err)
	}

	if pair == nil {
		t.Fatal("expected non-nil token pair")
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty access and refresh tokens")
	}
	if pair.TokenType != "Bearer" {
		t.Errorf("expected token type Bearer, got %s", pair.TokenType)
	}
	if pair.ExpiresIn != 86400 {
		t.Errorf("expected 86400 seconds access expiry, got %d", pair.ExpiresIn)
	}
	if pair.RefreshExpiresIn != 604800 {
		t.Errorf("expected 604800 seconds refresh expiry, got %d", pair.RefreshExpiresIn)
	}

	// Validate Access Token
	accessClaims, err := ValidateToken(cfg, pair.AccessToken)
	if err != nil {
		t.Fatalf("unexpected error validating access token: %v", err)
	}
	if accessClaims.UserID != userID {
		t.Errorf("expected user ID %v, got %v", userID, accessClaims.UserID)
	}
	if accessClaims.Email != email {
		t.Errorf("expected email %s, got %s", email, accessClaims.Email)
	}
	if accessClaims.SessionID != sessionID {
		t.Errorf("expected session ID %s, got %s", sessionID, accessClaims.SessionID)
	}
	if accessClaims.TokenType != constants.JWTTokenTypeAccess {
		t.Errorf("expected token type access, got %s", accessClaims.TokenType)
	}

	// Validate Refresh Token
	refreshClaims, err := ValidateToken(cfg, pair.RefreshToken)
	if err != nil {
		t.Fatalf("unexpected error validating refresh token: %v", err)
	}
	if refreshClaims.UserID != userID {
		t.Errorf("expected user ID %v, got %v", userID, refreshClaims.UserID)
	}
	if refreshClaims.TokenType != constants.JWTTokenTypeRefresh {
		t.Errorf("expected token type refresh, got %s", refreshClaims.TokenType)
	}

	// Invalid signature verification
	badCfg := cfg
	badCfg.JWTSecret = "wrong-secret-key"
	_, err = ValidateToken(badCfg, pair.AccessToken)
	if err == nil {
		t.Fatal("expected error validating token with wrong secret, got nil")
	}

	// Expired token verification
	expiredCfg := cfg
	expiredCfg.JWTAccessExpiration = -1 * time.Minute
	expiredPair, err := GenerateTokenPair(expiredCfg, userID, email, sessionID)
	if err != nil {
		t.Fatalf("unexpected error generating expired token: %v", err)
	}
	_, err = ValidateToken(cfg, expiredPair.AccessToken)
	if err == nil {
		t.Fatal("expected error validating expired token, got nil")
	}
}
