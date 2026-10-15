package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// ValidateFeedbackRequest normalizes and validates a feedback submission payload.
func ValidateFeedbackRequest(r *dtos.FeedbackRequest) error {
	r.Title = strings.TrimSpace(r.Title)
	r.Message = strings.TrimSpace(r.Message)
	r.Category = strings.ToLower(strings.TrimSpace(r.Category))

	r.SetDefaults()

	if r.UserName != nil {
		trimmed := strings.TrimSpace(*r.UserName)
		if trimmed == "" {
			r.UserName = nil
		} else {
			r.UserName = &trimmed
		}
	}

	if r.UserEmail != nil {
		trimmed := strings.TrimSpace(*r.UserEmail)
		if trimmed == "" {
			r.UserEmail = nil
		} else {
			r.UserEmail = &trimmed
		}
	}

	if r.IdempotencyKey != nil {
		trimmed := strings.TrimSpace(*r.IdempotencyKey)
		if trimmed == "" {
			r.IdempotencyKey = nil
		} else {
			r.IdempotencyKey = &trimmed
		}
	}

	return validation.ValidateStruct(r,
		validation.Field(&r.Title, validation.Required, validation.Length(4, 255)),
		validation.Field(&r.Message, validation.Required, validation.Length(10, 5000)),
		validation.Field(&r.Rating, validation.Required, validation.Min(1), validation.Max(5)),
		validation.Field(&r.Category, validation.Required, validation.In(
			constants.FeedbackCategoryBug,
			constants.FeedbackCategoryFeature,
			constants.FeedbackCategoryImprovement,
			constants.FeedbackCategoryPraise,
			constants.FeedbackCategoryGeneral,
		)),
		validation.Field(&r.UserEmail, is.EmailFormat),
		validation.Field(&r.UserName, validation.Length(1, 150)),
		validation.Field(&r.IdempotencyKey, validation.Length(1, 100)),
	)
}
