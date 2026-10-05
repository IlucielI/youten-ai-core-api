package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/dtos"
)

// ValidateWaitlistRequest normalizes and validates a bot waitlist signup payload.
func ValidateWaitlistRequest(r *dtos.WaitlistRequest) error {
	r.Email = strings.TrimSpace(r.Email)
	if r.Email == "" {
		return validation.Errors{
			"email": validation.NewError("validation_required", "email is required and cannot be blank"),
		}
	}
	return validation.ValidateStruct(r,
		validation.Field(&r.Email, validation.Required, is.EmailFormat, validation.Length(3, 255)),
		validation.Field(&r.Platform, validation.Length(0, 50)),
		validation.Field(&r.CompanySize, validation.Length(0, 50)),
	)
}
