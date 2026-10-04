package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/hasher"
	"code-base-golang/internal/pkg/jwt"
)

// Register creates a new user account with a hashed password and default daily quota.
func (s *Service) Register(ctx context.Context, req *dtos.RegisterRequest) (*dtos.UserResponse, error) {
	if s == nil || s.repo == nil {
		return nil, constants.ErrInternalServerError
	}
	if req == nil {
		return nil, constants.ErrBadRequest.WithMessage("registration payload is required")
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))

	// Check if user with this email already exists
	existingUser, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, s.wrapError(ctx, err)
	}
	if existingUser != nil {
		return nil, constants.ErrEmailAlreadyExists
	}

	// Hash password with bcrypt cost 12
	passwordHash, err := hasher.HashPassword(req.Password)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	user := &models.User{
		Email:         email,
		PasswordHash:  passwordHash,
		FullName:      strings.TrimSpace(req.FullName),
		Status:        constants.UserStatusActive,
		EmailVerified: false,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	dailyQuota := constants.DefaultUserDailyQuota
	if user.DailyQuotaOverride != nil {
		dailyQuota = *user.DailyQuotaOverride
	}

	return &dtos.UserResponse{
		ID:                 user.ID,
		Email:              user.Email,
		FullName:           user.FullName,
		Status:             user.Status,
		DailyQuota:         dailyQuota,
		DailyQuotaOverride: user.DailyQuotaOverride,
		EmailVerified:      user.EmailVerified,
		CreatedAt:          user.CreatedAt,
	}, nil
}

// Login authenticates a user by email and password, returning an access and refresh token pair.
func (s *Service) Login(ctx context.Context, req *dtos.LoginRequest) (*dtos.AuthResponse, error) {
	if s == nil || s.repo == nil {
		return nil, constants.ErrInternalServerError
	}
	if req == nil {
		return nil, constants.ErrBadRequest.WithMessage("login payload is required")
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))

	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidCredentials
		}
		return nil, s.wrapError(ctx, err)
	}

	if user == nil {
		return nil, constants.ErrInvalidCredentials
	}

	if user.Status != constants.UserStatusActive {
		return nil, constants.ErrInvalidCredentials
	}

	if !hasher.VerifyPassword(user.PasswordHash, req.Password) {
		return nil, constants.ErrInvalidCredentials
	}

	sessionID := uuid.New().String()
	tokenPair, err := jwt.GenerateTokenPair(s.cfg, user.ID, user.Email, sessionID)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// Persist refresh token hash in auth_tokens table
	refreshTokenHash := hasher.HashToken(tokenPair.RefreshToken)
	authToken := &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeRefresh,
		TokenHash: refreshTokenHash,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiration),
	}

	if err := s.repo.CreateAuthToken(ctx, authToken); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	dailyQuota := constants.DefaultUserDailyQuota
	if user.DailyQuotaOverride != nil {
		dailyQuota = *user.DailyQuotaOverride
	}

	return &dtos.AuthResponse{
		AccessToken:      tokenPair.AccessToken,
		RefreshToken:     tokenPair.RefreshToken,
		TokenType:        tokenPair.TokenType,
		ExpiresIn:        tokenPair.ExpiresIn,
		RefreshExpiresIn: tokenPair.RefreshExpiresIn,
		User: dtos.UserResponse{
			ID:                 user.ID,
			Email:              user.Email,
			FullName:           user.FullName,
			Status:             user.Status,
			DailyQuota:         dailyQuota,
			DailyQuotaOverride: user.DailyQuotaOverride,
			EmailVerified:      user.EmailVerified,
			CreatedAt:          user.CreatedAt,
		},
	}, nil
}

// RefreshToken rotates session tokens by validating the active refresh token, revoking it, and issuing a new token pair.
func (s *Service) RefreshToken(ctx context.Context, req *dtos.RefreshTokenRequest) (*dtos.AuthResponse, error) {
	if s == nil || s.repo == nil {
		return nil, constants.ErrInternalServerError
	}
	if req == nil {
		return nil, constants.ErrBadRequest.WithMessage("refresh token payload is required")
	}

	rawToken := strings.TrimSpace(req.RefreshToken)
	if rawToken == "" {
		return nil, constants.ErrInvalidToken
	}

	// 1. Validate JWT structure and signature
	claims, err := jwt.ValidateToken(s.cfg, rawToken)
	if err != nil || claims == nil || claims.TokenType != "refresh" {
		return nil, constants.ErrInvalidToken
	}

	// 2. Lookup unrevoked, unexpired token in database by SHA-256 hash
	tokenHash := hasher.HashToken(rawToken)
	authToken, err := s.repo.FindAuthTokenByHashAndType(ctx, tokenHash, constants.AuthTokenTypeRefresh)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidToken
		}
		return nil, s.wrapError(ctx, err)
	}
	if authToken == nil {
		return nil, constants.ErrInvalidToken
	}

	// 3. Retrieve associated user and verify active status
	user, err := s.repo.FindUserByID(ctx, authToken.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constants.ErrInvalidToken
		}
		return nil, s.wrapError(ctx, err)
	}
	if user == nil || user.Status != constants.UserStatusActive {
		// Invalidate token for inactive user
		_ = s.repo.RevokeAuthToken(ctx, authToken.ID)
		return nil, constants.ErrInvalidToken
	}

	// 4. Generate fresh token pair with a new session ID
	sessionID := uuid.New().String()
	tokenPair, err := jwt.GenerateTokenPair(s.cfg, user.ID, user.Email, sessionID)
	if err != nil {
		return nil, s.wrapError(ctx, err)
	}

	// 5. Atomically rotate tokens: revoke old refresh token and insert new refresh token in transaction
	newTokenHash := hasher.HashToken(tokenPair.RefreshToken)
	newAuthToken := &models.AuthToken{
		UserID:    user.ID,
		Type:      constants.AuthTokenTypeRefresh,
		TokenHash: newTokenHash,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiration),
	}

	if err := s.repo.RotateAuthToken(ctx, authToken.ID, newAuthToken); err != nil {
		return nil, s.wrapError(ctx, err)
	}

	dailyQuota := constants.DefaultUserDailyQuota
	if user.DailyQuotaOverride != nil {
		dailyQuota = *user.DailyQuotaOverride
	}

	return &dtos.AuthResponse{
		AccessToken:      tokenPair.AccessToken,
		RefreshToken:     tokenPair.RefreshToken,
		TokenType:        tokenPair.TokenType,
		ExpiresIn:        tokenPair.ExpiresIn,
		RefreshExpiresIn: tokenPair.RefreshExpiresIn,
		User: dtos.UserResponse{
			ID:                 user.ID,
			Email:              user.Email,
			FullName:           user.FullName,
			Status:             user.Status,
			DailyQuota:         dailyQuota,
			DailyQuotaOverride: user.DailyQuotaOverride,
			EmailVerified:      user.EmailVerified,
			CreatedAt:          user.CreatedAt,
		},
	}, nil
}
