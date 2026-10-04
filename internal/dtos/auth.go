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
