package jwt

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// UserRoleClaims contains optional user RBAC role code, permissions, and daily quota.
type UserRoleClaims struct {
	RoleCode    string
	Permissions []string
	DailyQuota  int
}

// CustomClaims encapsulates application-specific payload fields in standard JWT claims.
type CustomClaims struct {
	UserID      uuid.UUID              `json:"sub"`
	SessionID   string                 `json:"sid"`
	Email       string                 `json:"email"`
	RoleCode    string                 `json:"role,omitempty"`
	Permissions []string               `json:"permissions,omitempty"`
	DailyQuota  int                    `json:"daily_quota,omitempty"`
	TokenType   constants.JWTTokenType `json:"type"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// HasPermission checks if claims contain required permission (exact, wildcard *, or domain prefix *).
func (c *CustomClaims) HasPermission(requiredPerm string) bool {
	if c == nil {
		return false
	}
	requiredPerm = strings.TrimSpace(requiredPerm)
	if requiredPerm == "" {
		return true
	}
	parts := strings.Split(requiredPerm, ":")
	domainPrefix := ""
	if len(parts) > 1 {
		domainPrefix = parts[0] + ":*"
	}

	for _, p := range c.Permissions {
		p = strings.TrimSpace(p)
		if p == "*" || p == requiredPerm || (domainPrefix != "" && p == domainPrefix) {
			return true
		}
	}
	return false
}

// GenerateTokenPair signs and returns a new Access Token and Refresh Token pair.
func GenerateTokenPair(cfg config.Config, userID uuid.UUID, email string, sessionID string, roleClaims ...UserRoleClaims) (*dtos.TokenResponse, error) {
	now := time.Now()
	accessExp := now.Add(cfg.JWTAccessExpiration)
	refreshExp := now.Add(cfg.JWTRefreshExpiration)

	var roleCode string
	var permissions []string
	var dailyQuota int
	if len(roleClaims) > 0 {
		roleCode = roleClaims[0].RoleCode
		permissions = roleClaims[0].Permissions
		dailyQuota = roleClaims[0].DailyQuota
	}

	// 1. Access Token
	accessClaims := CustomClaims{
		UserID:      userID,
		SessionID:   sessionID,
		Email:       email,
		RoleCode:    roleCode,
		Permissions: permissions,
		DailyQuota:  dailyQuota,
		TokenType:   constants.JWTTokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    cfg.AppName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExp),
		},
	}
	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessToken, err := accessTokenObj.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// 2. Refresh Token
	refreshClaims := CustomClaims{
		UserID:    userID,
		SessionID: sessionID,
		Email:     email,
		TokenType: constants.JWTTokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    cfg.AppName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExp),
		},
	}
	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshToken, err := refreshTokenObj.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return &dtos.TokenResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresIn:        int64(cfg.JWTAccessExpiration.Seconds()),
		RefreshExpiresIn: int64(cfg.JWTRefreshExpiration.Seconds()),
	}, nil
}

// ValidateToken parses and verifies the signature and expiration of a JWT token string.
func ValidateToken(cfg config.Config, tokenStr string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, constants.ErrInvalidToken
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, constants.ErrInvalidToken
	}

	if claims.TokenType != constants.JWTTokenTypeAccess && claims.TokenType != constants.JWTTokenTypeRefresh {
		return nil, constants.ErrInvalidToken
	}

	return claims, nil
}

// AdminClaims encapsulates admin identity payload fields in standard JWT claims.
type AdminClaims struct {
	AdminID   uuid.UUID               `json:"sub"`
	Username  string                  `json:"username"`
	TokenType constants.JWTTokenType `json:"type"` // "admin_access"
	jwt.RegisteredClaims
}

// GenerateAdminToken signs and returns a new Admin Access Token for identity verification.
func GenerateAdminToken(cfg config.Config, adminID uuid.UUID, username string) (string, error) {
	now := time.Now()
	exp := now.Add(cfg.JWTAccessExpiration)

	claims := AdminClaims{
		AdminID:   adminID,
		Username:  username,
		TokenType: constants.JWTTokenTypeAdminAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   adminID.String(),
			Issuer:    cfg.AppName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tokenObj.SignedString([]byte(cfg.JWTSecret))
}

// ValidateAdminToken parses and verifies the signature and expiration of an Admin JWT token string.
func ValidateAdminToken(cfg config.Config, tokenStr string) (*AdminClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AdminClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, constants.ErrInvalidToken
	}

	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid {
		return nil, constants.ErrInvalidToken
	}

	if claims.TokenType != constants.JWTTokenTypeAdminAccess {
		return nil, constants.ErrInvalidToken
	}

	return claims, nil
}

// AnonClaims encapsulates guest session token claims with allowed scopes and 7-day TTL.
type AnonClaims struct {
	SessionID uuid.UUID              `json:"sub"`
	ClientID  string                 `json:"client_id"`
	TokenType constants.JWTTokenType `json:"type"` // "anon_access"
	Scopes    []string               `json:"scopes"`
	jwt.RegisteredClaims
}

// HasScope checks if the anonymous claims contain a specific scope or wildcard.
func (c *AnonClaims) HasScope(requiredScope string) bool {
	if c == nil {
		return false
	}
	for _, s := range c.Scopes {
		if s == string(constants.ScopeWildcard) || s == requiredScope {
			return true
		}
	}
	return false
}

// GenerateAnonToken signs and returns a new scoped Anonymous Session Token (7-day TTL).
func GenerateAnonToken(cfg config.Config, sessionID uuid.UUID, clientID string, scopes []string) (string, int64, error) {
	now := time.Now()
	ttl := 7 * 24 * time.Hour // 7 days (168 hours)
	exp := now.Add(ttl)

	claims := AnonClaims{
		SessionID: sessionID,
		ClientID:  clientID,
		TokenType: constants.JWTTokenTypeAnonAccess,
		Scopes:    scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sessionID.String(),
			Issuer:    cfg.AppName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := tokenObj.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", 0, fmt.Errorf("failed to sign anon token: %w", err)
	}
	return tokenStr, int64(ttl.Seconds()), nil
}

// ValidateAnonToken parses and validates an anonymous session token string.
func ValidateAnonToken(cfg config.Config, tokenStr string) (*AnonClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AnonClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, constants.ErrInvalidToken
	}

	claims, ok := token.Claims.(*AnonClaims)
	if !ok || !token.Valid {
		return nil, constants.ErrInvalidToken
	}

	if claims.TokenType != constants.JWTTokenTypeAnonAccess {
		return nil, constants.ErrInvalidToken
	}

	return claims, nil
}

