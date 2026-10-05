package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"code-base-golang/internal/dtos"
)

// ValidateRecordingChatRequest validates an interactive RAG chat payload.
func ValidateRecordingChatRequest(r *dtos.RecordingChatRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.Message,
			validation.Required,
			validation.Length(1, 4000),
			validation.By(func(value interface{}) error {
				s, ok := value.(string)
				if !ok || strings.TrimSpace(s) == "" {
					return validation.NewError("validation_required", "message cannot be empty or blank")
				}
				return nil
			}),
		),
	)
}
