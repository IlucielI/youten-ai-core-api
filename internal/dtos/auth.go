package dtos

import (
	"time"

	"github.com/google/uuid"
)

// RegisterRequest defines the input payload for user registration.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

// UserResponse represents the public user response representation.
type UserResponse struct {
	ID                 uuid.UUID `json:"id"`
	Email              string    `json:"email"`
	FullName           string    `json:"full_name"`
	Status             string    `json:"status"`
	DailyQuota         int       `json:"daily_quota"`
	DailyQuotaOverride *int      `json:"daily_quota_override,omitempty"`
	EmailVerified      bool      `json:"email_verified"`
	CreatedAt          time.Time `json:"created_at"`
}

// UserProfileResponse represents the detailed authenticated user profile with live daily quota calculations.
type UserProfileResponse struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	FullName       string    `json:"full_name"`
	Status         string    `json:"status"`
	DailyQuota     int       `json:"daily_quota"`
	QuotaUsedToday int       `json:"quota_used_today"`
	QuotaRemaining int       `json:"quota_remaining"`
	EmailVerified  bool      `json:"email_verified"`
	CreatedAt      time.Time `json:"created_at"`
}

// UpdateProfileRequest defines the input payload for updating user profile attributes.
type UpdateProfileRequest struct {
	FullName string `json:"full_name"`
}

// ChangePasswordRequest defines the input payload for changing an authenticated user's password.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// LoginRequest defines the input payload for user authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the response envelope containing tokens and authenticated user info.
type AuthResponse struct {
	AccessToken      string       `json:"access_token"`
	RefreshToken     string       `json:"refresh_token"`
	TokenType        string       `json:"token_type"`
	ExpiresIn        int64        `json:"expires_in"`
	RefreshExpiresIn int64        `json:"refresh_expires_in"`
	User             UserResponse `json:"user"`
}

// RefreshTokenRequest defines the input payload for rotating session tokens.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest defines the input payload for terminating a session via refresh token.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// ForgotPasswordRequest defines the input payload for requesting a password reset email.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest defines the input payload for confirming a password reset with a valid token.
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}
