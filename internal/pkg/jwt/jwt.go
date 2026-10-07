package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// CustomClaims encapsulates application-specific payload fields in standard JWT claims.
type CustomClaims struct {
	UserID    uuid.UUID `json:"sub"`
	SessionID string    `json:"sid"`
	Email     string    `json:"email"`
	TokenType constants.JWTTokenType `json:"type"` // "access" or "refresh"
	jwt.RegisteredClaims
}

// GenerateTokenPair signs and returns a new Access Token and Refresh Token pair.
func GenerateTokenPair(cfg config.Config, userID uuid.UUID, email string, sessionID string) (*dtos.TokenResponse, error) {
	now := time.Now()
	accessExp := now.Add(cfg.JWTAccessExpiration)
	refreshExp := now.Add(cfg.JWTRefreshExpiration)

	// 1. Access Token
	accessClaims := CustomClaims{
		UserID:    userID,
		SessionID: sessionID,
		Email:     email,
		TokenType: constants.JWTTokenTypeAccess,
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
