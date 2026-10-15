package validations

import (
	"errors"
	"path/filepath"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
)

// SupportedFeedbackImageMIMEs defines allowed MIME types for feedback screenshots and image attachments.
var SupportedFeedbackImageMIMEs = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/jpg":  true,
	"image/webp": true,
	"image/gif":  true,
}

// SupportedFeedbackImageExts maps file extensions to fallback MIME types.
var SupportedFeedbackImageExts = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
}

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

	for i := range r.Attachments {
		r.Attachments[i].URL = strings.TrimSpace(r.Attachments[i].URL)
		r.Attachments[i].Filename = strings.TrimSpace(r.Attachments[i].Filename)
		r.Attachments[i].ContentType = strings.TrimSpace(r.Attachments[i].ContentType)
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
		validation.Field(&r.Attachments, validation.Length(0, constants.MaxFeedbackAttachmentsCount), validation.Each(
			validation.By(func(value interface{}) error {
				att, ok := value.(dtos.FeedbackAttachment)
				if !ok {
					return errors.New("invalid attachment type")
				}
				return validation.ValidateStruct(&att,
					validation.Field(&att.URL, validation.Required, validation.Length(1, 2048)),
					validation.Field(&att.Filename, validation.Required, validation.Length(1, 255)),
				)
			}),
		)),
	)
}

// ValidateFeedbackPresignRequest validates an upload presign request for feedback screenshots.
func ValidateFeedbackPresignRequest(r *dtos.FeedbackPresignRequest) error {
	r.Filename = strings.TrimSpace(r.Filename)
	r.ContentType = strings.ToLower(strings.TrimSpace(r.ContentType))

	cleanFilename := filepath.Base(r.Filename)
	if cleanFilename == "." || cleanFilename == "/" || cleanFilename == "" {
		return errors.New("filename cannot be empty or invalid")
	}

	// Resolve MIME from extension if content type was generic or blank
	ext := strings.ToLower(filepath.Ext(cleanFilename))
	if r.ContentType == "" || r.ContentType == "application/octet-stream" {
		if mappedMIME, ok := SupportedFeedbackImageExts[ext]; ok {
			r.ContentType = mappedMIME
		}
	}

	if !SupportedFeedbackImageMIMEs[r.ContentType] {
		return errors.New("unsupported image format; allowed formats: png, jpeg, webp, gif")
	}

	return validation.ValidateStruct(r,
		validation.Field(&r.Filename, validation.Required, validation.Length(1, 255)),
		validation.Field(&r.ContentType, validation.Required, validation.Length(1, 100)),
		validation.Field(&r.FileSizeBytes, validation.Max(int64(constants.MaxFeedbackAttachmentSizeBytes))),
	)
}
