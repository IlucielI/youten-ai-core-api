package dtos

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

// WaitlistRequest represents the input payload for joining the meeting voice bot beta waitlist.
type WaitlistRequest struct {
	Email       string `json:"email"`
	Platform    string `json:"platform"`
	CompanySize string `json:"company_size"`
}

// Validate validates the WaitlistRequest fields.
func (r WaitlistRequest) Validate() error {
	trimmedEmail := strings.TrimSpace(r.Email)
	if trimmedEmail == "" {
		return validation.Errors{
			"email": validation.NewError("validation_required", "email is required and cannot be blank"),
		}
	}

	return validation.ValidateStruct(&r,
		validation.Field(&r.Email, validation.Required, is.EmailFormat, validation.Length(3, 255)),
		validation.Field(&r.Platform, validation.Length(0, 50)),
		validation.Field(&r.CompanySize, validation.Length(0, 50)),
	)
}

// WaitlistResponse represents the response envelope confirming waitlist registration.
type WaitlistResponse struct {
	Email       string `json:"email"`
	Platform    string `json:"platform"`
	CompanySize string `json:"company_size"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}
