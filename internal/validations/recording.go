package validations

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/dtos"
)

// ValidatePresignUploadRequest validates a presigned upload request.
func ValidatePresignUploadRequest(r *dtos.PresignUploadRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.Filename, validation.Required, validation.Length(1, 255)),
	)
}

// ValidateUploadRecordingRequest validates an upload confirmation payload.
func ValidateUploadRecordingRequest(r *dtos.UploadRecordingRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.Filename, validation.Required, validation.Length(1, 255)),
		validation.Field(&r.Title, validation.Length(0, 255)),
		validation.Field(&r.Template, validation.Length(0, 100)),
		validation.Field(&r.Language, validation.Length(0, 50)),
	)
}

// ValidateImportURLRequest validates a link-import payload.
func ValidateImportURLRequest(r *dtos.ImportURLRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.URL, validation.Required, is.URL),
		validation.Field(&r.Title, validation.Length(0, 255)),
		validation.Field(&r.Template, validation.Length(0, 100)),
		validation.Field(&r.Language, validation.Length(0, 50)),
	)
}

// ValidateClaimRecordingRequest validates a single guest-recording claim payload.
func ValidateClaimRecordingRequest(r *dtos.ClaimRecordingRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.OwnershipToken, validation.Required, validation.Length(1, 255)),
	)
}

// ValidateBulkClaimRequest validates a bulk guest-recording claim payload.
func ValidateBulkClaimRequest(r *dtos.BulkClaimRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.Tokens, validation.Required, validation.Length(1, 100), validation.Each(validation.Required, validation.Length(1, 255))),
	)
}

// ValidateShareToggleRequest validates a share-toggle payload.
func ValidateShareToggleRequest(r *dtos.ShareToggleRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.IsShareEnabled, validation.NotNil),
	)
}

// ValidateUpdateSpeakersRequest validates a speaker rename mapping.
func ValidateUpdateSpeakersRequest(r *dtos.UpdateSpeakersRequest) error {
	if len(r.Speakers) == 0 {
		return validation.NewError("validation_required", "speakers mapping cannot be empty")
	}
	for label, name := range r.Speakers {
		trimmedLabel := strings.TrimSpace(label)
		if trimmedLabel == "" {
			return validation.NewError("validation_required", "speaker label cannot be blank")
		}
		if len(trimmedLabel) > 50 {
			return validation.NewError("validation_length", "speaker label cannot exceed 50 characters")
		}
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			return validation.NewError("validation_required", "speaker name cannot be blank")
		}
		if len(trimmedName) > 100 {
			return validation.NewError("validation_length", "speaker name cannot exceed 100 characters")
		}
	}
	return nil
}

// ValidateRegenerateSummaryRequest validates a summary regeneration payload.
func ValidateRegenerateSummaryRequest(r *dtos.RegenerateSummaryRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.TemplateCategory, validation.Length(0, 100)),
		validation.Field(&r.CustomAngle, validation.NilOrNotEmpty, validation.Length(0, 2000)),
	)
}

// ValidateCreateCommentRequest validates an inline comment creation payload.
func ValidateCreateCommentRequest(r *dtos.CreateCommentRequest) error {
	return validation.ValidateStruct(r,
		validation.Field(&r.CommentText, validation.Required, validation.Length(1, 5000)),
		validation.Field(&r.AuthorName, validation.Length(0, 100)),
		validation.Field(&r.TimestampSec, validation.Min(0.0)),
	)
}
