package dtos

import (
	"regexp"
	"strings"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/google/uuid"
)

var (
	digitRegex = regexp.MustCompile(`[0-9]`)
)

// RegisterRequest defines the input payload for user registration.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

// Validate performs structural and semantic validation on RegisterRequest.
func (r *RegisterRequest) Validate() error {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.FullName = strings.TrimSpace(r.FullName)

	return validation.ValidateStruct(r,
		validation.Field(&r.Email,
			validation.Required.Error("email is required"),
			is.EmailFormat.Error("invalid email format"),
		),
		validation.Field(&r.FullName,
			validation.Required.Error("full_name is required"),
			validation.Length(2, 100).Error("full_name must be between 2 and 100 characters"),
		),
		validation.Field(&r.Password,
			validation.Required.Error("password is required"),
			validation.Length(8, 72).Error("password must be between 8 and 72 characters"),
			validation.Match(digitRegex).Error("password must contain at least one digit"),
		),
	)
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

// Validate performs structural and semantic validation on UpdateProfileRequest.
func (r *UpdateProfileRequest) Validate() error {
	r.FullName = strings.TrimSpace(r.FullName)

	return validation.ValidateStruct(r,
		validation.Field(&r.FullName,
			validation.Required.Error("full name is required"),
			validation.Length(2, 100).Error("full name must be between 2 and 100 characters"),
		),
	)
}


// LoginRequest defines the input payload for user authentication.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Validate performs structural and semantic validation on LoginRequest.
func (r *LoginRequest) Validate() error {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))

	return validation.ValidateStruct(r,
		validation.Field(&r.Email,
			validation.Required.Error("email is required"),
			is.EmailFormat.Error("invalid email format"),
		),
		validation.Field(&r.Password,
			validation.Required.Error("password is required"),
		),
	)
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

// Validate performs structural and semantic validation on RefreshTokenRequest.
func (r *RefreshTokenRequest) Validate() error {
	r.RefreshToken = strings.TrimSpace(r.RefreshToken)

	return validation.ValidateStruct(r,
		validation.Field(&r.RefreshToken,
			validation.Required.Error("refresh_token is required"),
		),
	)
}

// LogoutRequest defines the input payload for terminating a session via refresh token.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Validate performs structural and semantic validation on LogoutRequest.
func (r *LogoutRequest) Validate() error {
	r.RefreshToken = strings.TrimSpace(r.RefreshToken)

	return validation.ValidateStruct(r,
		validation.Field(&r.RefreshToken,
			validation.Required.Error("refresh_token is required"),
		),
	)
}

// ForgotPasswordRequest defines the input payload for requesting a password reset email.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// Validate performs structural and semantic validation on ForgotPasswordRequest.
func (r *ForgotPasswordRequest) Validate() error {
	return validation.ValidateStruct(r,
		validation.Field(&r.Email,
			validation.Required.Error("email is required"),
			validation.By(func(value interface{}) error {
				s, ok := value.(string)
				if !ok {
					return nil
				}
				trimmed := strings.TrimSpace(s)
				if trimmed == "" {
					return validation.NewError("validation_required", "email is required")
				}
				if err := is.EmailFormat.Validate(trimmed); err != nil {
					return validation.NewError("validation_is_email", "invalid email format")
				}
				return nil
			}),
		),
	)
}

// ResetPasswordRequest defines the input payload for confirming a password reset with a valid token.
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// Validate performs structural and semantic validation on ResetPasswordRequest.
func (r *ResetPasswordRequest) Validate() error {
	return validation.ValidateStruct(r,
		validation.Field(&r.Token,
			validation.Required.Error("token is required"),
		),
		validation.Field(&r.NewPassword,
			validation.Required.Error("new_password is required"),
			validation.Length(8, 72).Error("new_password must be between 8 and 72 characters"),
			validation.Match(digitRegex).Error("new_password must contain at least one digit"),
		),
	)
}
