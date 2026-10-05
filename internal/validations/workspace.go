package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"code-base-golang/internal/dtos"
)

// DefaultSemanticSearchLimit is applied when a search request omits a positive limit.
const DefaultSemanticSearchLimit = 10

// ValidateSemanticSearchQuery normalizes and validates a cross-meeting search query.
// It applies the default result limit when none is provided; the maximum is enforced
// by the validation rules below.
func ValidateSemanticSearchQuery(q *dtos.SemanticSearchQuery) error {
	q.Q = strings.TrimSpace(q.Q)
	if q.Q == "" {
		return validation.Errors{
			"q": validation.NewError("validation_required", "search query is required and cannot be blank"),
		}
	}
	if q.Limit == 0 {
		q.Limit = DefaultSemanticSearchLimit
	}
	return validation.ValidateStruct(q,
		validation.Field(&q.Q, validation.Required, validation.Length(1, 1000)),
		validation.Field(&q.Limit, validation.Min(0), validation.Max(50)),
		validation.Field(&q.Threshold, validation.Min(0.0), validation.Max(1.0)),
	)
}

// ValidateWorkspaceAskRequest normalizes and validates a workspace memory chat payload.
func ValidateWorkspaceAskRequest(r *dtos.WorkspaceAskRequest) error {
	r.Question = strings.TrimSpace(r.Question)
	if r.Question == "" {
		return validation.Errors{
			"question": validation.NewError("validation_required", "question is required and cannot be blank"),
		}
	}
	return validation.ValidateStruct(r,
		validation.Field(&r.Question, validation.Required, validation.Length(1, 4000)),
	)
}
