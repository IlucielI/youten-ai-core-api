package services

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/hasher"
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
