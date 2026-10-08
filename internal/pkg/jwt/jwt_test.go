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

func TestAdminToken(t *testing.T) {
	cfg := config.Config{
		AppName:             "YoutenAI",
		JWTSecret:           "super-secret-admin-jwt-key-32bytes!",
		JWTAccessExpiration: 15 * time.Minute,
	}

	adminID := uuid.New()
	username := "admintest"

	tokenStr, err := GenerateAdminToken(cfg, adminID, username)
	if err != nil {
		t.Fatalf("unexpected error generating admin token: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected non-empty admin token string")
	}

	claims, err := ValidateAdminToken(cfg, tokenStr)
	if err != nil {
		t.Fatalf("unexpected error validating admin token: %v", err)
	}
	if claims.AdminID != adminID {
		t.Errorf("expected admin ID %s, got %s", adminID, claims.AdminID)
	}
	if claims.Username != username {
		t.Errorf("expected username %s, got %s", username, claims.Username)
	}
	if claims.TokenType != constants.JWTTokenTypeAdminAccess {
		t.Errorf("expected token type %s, got %s", constants.JWTTokenTypeAdminAccess, claims.TokenType)
	}

	// Validate with wrong secret fails
	badCfg := cfg
	badCfg.JWTSecret = "wrong-admin-secret"
	_, err = ValidateAdminToken(badCfg, tokenStr)
	if err == nil {
		t.Fatal("expected error validating admin token with wrong secret, got nil")
	}

	// Validate expired admin token fails
	expiredCfg := cfg
	expiredCfg.JWTAccessExpiration = -1 * time.Minute
	expiredToken, err := GenerateAdminToken(expiredCfg, adminID, username)
	if err != nil {
		t.Fatalf("unexpected error generating expired admin token: %v", err)
	}
	_, err = ValidateAdminToken(cfg, expiredToken)
	if err == nil {
		t.Fatal("expected error validating expired admin token, got nil")
	}

	// Passing standard access token to ValidateAdminToken fails
	pair, err := GenerateTokenPair(cfg, adminID, "admin@test.com", "sess-1")
	if err != nil {
		t.Fatalf("unexpected error generating user token pair: %v", err)
	}
	_, err = ValidateAdminToken(cfg, pair.AccessToken)
	if err == nil {
		t.Fatal("expected error validating user token with ValidateAdminToken, got nil")
	}
}

func TestGenerateAndValidateAnonToken(t *testing.T) {
	cfg := config.Config{
		AppName:   "youten-test",
		JWTSecret: "test-secret-key-32bytes-long-super!",
	}

	sessionID := uuid.New()
	clientID := "client-app"
	scopes := []string{"recordings:create", "recordings:read", "auth:claim"}

	tokenStr, expiresIn, err := GenerateAnonToken(cfg, sessionID, clientID, scopes)
	if err != nil {
		t.Fatalf("unexpected error generating anon token: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected non-empty anon token string")
	}
	if expiresIn != 7*24*3600 {
		t.Errorf("expected 7 days expiresIn (%d), got %d", 7*24*3600, expiresIn)
	}

	claims, err := ValidateAnonToken(cfg, tokenStr)
	if err != nil {
		t.Fatalf("unexpected error validating anon token: %v", err)
	}
	if claims.SessionID != sessionID {
		t.Errorf("expected session ID %s, got %s", sessionID, claims.SessionID)
	}
	if claims.ClientID != clientID {
		t.Errorf("expected client ID %s, got %s", clientID, claims.ClientID)
	}
	if claims.TokenType != constants.JWTTokenTypeAnonAccess {
		t.Errorf("expected token type %s, got %s", constants.JWTTokenTypeAnonAccess, claims.TokenType)
	}
	if len(claims.Scopes) != len(scopes) {
		t.Errorf("expected %d scopes, got %d", len(scopes), len(claims.Scopes))
	}

	// Scope validation tests
	if !claims.HasScope("recordings:create") {
		t.Error("expected HasScope('recordings:create') to be true")
	}
	if !claims.HasScope("recordings:read") {
		t.Error("expected HasScope('recordings:read') to be true")
	}
	if claims.HasScope("recordings:share") {
		t.Error("expected HasScope('recordings:share') to be false")
	}

	// Wildcard scope test
	wildcardClaims := &AnonClaims{
		Scopes: []string{"*"},
	}
	if !wildcardClaims.HasScope("any:permission") {
		t.Error("expected wildcard to grant any:permission")
	}

	// Wrong secret test
	badCfg := cfg
	badCfg.JWTSecret = "wrong-jwt-secret-12345"
	_, err = ValidateAnonToken(badCfg, tokenStr)
	if err == nil {
		t.Fatal("expected error validating anon token with wrong secret, got nil")
	}

	// Passing anon token to ValidateToken (user claims) fails
	_, err = ValidateToken(cfg, tokenStr)
	if err == nil {
		t.Fatal("expected error validating anon token as user token, got nil")
	}

	// Passing anon token to ValidateAdminToken fails
	_, err = ValidateAdminToken(cfg, tokenStr)
	if err == nil {
		t.Fatal("expected error validating anon token as admin token, got nil")
	}
}


