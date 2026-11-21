package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"code-base-golang/internal/dtos"
)

// ValidateCreateFolderRequest validates folder creation input.
func ValidateCreateFolderRequest(r *dtos.CreateFolderRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Color == "" {
		r.Color = "#4F46E5"
	}
	r.Color = strings.TrimSpace(r.Color)

	return validation.ValidateStruct(r,
		validation.Field(&r.Name, validation.Required, validation.Length(1, 255)),
		validation.Field(&r.Color, validation.Required, validation.Length(1, 50)),
	)
}

// ValidateUpdateFolderRequest validates folder update input.
func ValidateUpdateFolderRequest(r *dtos.UpdateFolderRequest) error {
	if r.Name != nil {
		trimmed := strings.TrimSpace(*r.Name)
		r.Name = &trimmed
	}
	if r.Color != nil {
		trimmed := strings.TrimSpace(*r.Color)
		r.Color = &trimmed
	}

	return validation.ValidateStruct(r,
		validation.Field(&r.Name, validation.NilOrNotEmpty, validation.Length(1, 255)),
		validation.Field(&r.Color, validation.NilOrNotEmpty, validation.Length(1, 50)),
	)
}
