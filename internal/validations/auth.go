package validations

import (
	"regexp"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/dtos"
)

var digitRegex = regexp.MustCompile(`[0-9]`)

// ValidateRegisterRequest normalizes and validates a registration payload.
func ValidateRegisterRequest(r *dtos.RegisterRequest) error {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.FullName = strings.TrimSpace(r.FullName)
	r.AnonToken = strings.TrimSpace(r.AnonToken)

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
		validation.Field(&r.AnonToken,
			validation.Length(0, 1024).Error("anon_token cannot exceed 1024 characters"),
		),
	)
}

// ValidateUpdateProfileRequest normalizes and validates a profile update payload.
func ValidateUpdateProfileRequest(r *dtos.UpdateProfileRequest) error {
	r.FullName = strings.TrimSpace(r.FullName)

	return validation.ValidateStruct(r,
		validation.Field(&r.FullName,
			validation.Required.Error("full name is required"),
			validation.Length(2, 100).Error("full name must be between 2 and 100 characters"),
		),
	)
}

// ValidateChangePasswordRequest validates a password change payload.
func ValidateChangePasswordRequest(r *dtos.ChangePasswordRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.OldPassword,
			validation.Required.Error("old password is required"),
		),
		validation.Field(&r.NewPassword,
			validation.Required.Error("new password is required"),
			validation.Length(8, 72).Error("new password must be between 8 and 72 characters"),
			validation.Match(digitRegex).Error("new password must contain at least one digit"),
			validation.By(func(value interface{}) error {
				if newPassword, ok := value.(string); ok && newPassword == r.OldPassword {
					return validation.NewError("validation_password_unchanged", "new password cannot be the same as current password")
				}
				return nil
			}),
		),
	)
}

// ValidateLoginRequest normalizes and validates a login payload.
func ValidateLoginRequest(r *dtos.LoginRequest) error {
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.AnonToken = strings.TrimSpace(r.AnonToken)

	return validation.ValidateStruct(r,
		validation.Field(&r.Email,
			validation.Required.Error("email is required"),
			is.EmailFormat.Error("invalid email format"),
		),
		validation.Field(&r.Password,
			validation.Required.Error("password is required"),
		),
		validation.Field(&r.AnonToken,
			validation.Length(0, 1024).Error("anon_token cannot exceed 1024 characters"),
		),
	)
}

// ValidateRefreshTokenRequest normalizes and validates a refresh token payload.
func ValidateRefreshTokenRequest(r *dtos.RefreshTokenRequest) error {
	r.RefreshToken = strings.TrimSpace(r.RefreshToken)

	return validation.ValidateStruct(r,
		validation.Field(&r.RefreshToken,
			validation.Required.Error("refresh_token is required"),
		),
	)
}

// ValidateLogoutRequest normalizes and validates a logout payload.
func ValidateLogoutRequest(r *dtos.LogoutRequest) error {
	r.RefreshToken = strings.TrimSpace(r.RefreshToken)

	return validation.ValidateStruct(r,
		validation.Field(&r.RefreshToken,
			validation.Required.Error("refresh_token is required"),
		),
	)
}

// ValidateForgotPasswordRequest validates a password reset request payload.
func ValidateForgotPasswordRequest(r *dtos.ForgotPasswordRequest) error {
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

// ValidateResetPasswordRequest validates a password reset confirmation payload.
func ValidateResetPasswordRequest(r *dtos.ResetPasswordRequest) error {
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
