package dtos

import (
	"time"

	"github.com/google/uuid"
)

// FeedbackAttachment carries metadata of a file or screenshot attached to feedback.
type FeedbackAttachment struct {
	URL           string `json:"url"`
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

// FeedbackRequest defines the payload for submitting user feedback, bug reports, or suggestions.
type FeedbackRequest struct {
	Title          string                 `json:"title"`
	Category       string                 `json:"category"`
	Rating         int                    `json:"rating"`
	Message        string                 `json:"message"`
	UserName       *string                `json:"user_name,omitempty"`
	UserEmail      *string                `json:"user_email,omitempty"`
	IdempotencyKey *string                `json:"idempotency_key,omitempty"`
	Attachments    []FeedbackAttachment   `json:"attachments,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SetDefaults sets sensible fallback defaults for feedback request fields.
func (r *FeedbackRequest) SetDefaults() {
	if r.Category == "" {
		r.Category = "general"
	}
	if r.Rating <= 0 {
		r.Rating = 5
	}
	if r.Attachments == nil {
		r.Attachments = make([]FeedbackAttachment, 0)
	}
}

// FeedbackResponse represents the returned feedback confirmation data.
type FeedbackResponse struct {
	ID             uuid.UUID              `json:"id"`
	Title          string                 `json:"title"`
	Category       string                 `json:"category"`
	Rating         int                    `json:"rating"`
	Message        string                 `json:"message"`
	Status         string                 `json:"status"`
	UserName       *string                `json:"user_name,omitempty"`
	UserEmail      *string                `json:"user_email,omitempty"`
	IdempotencyKey *string                `json:"idempotency_key,omitempty"`
	Attachments    []FeedbackAttachment   `json:"attachments,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// FeedbackPresignRequest carries metadata for generating a presigned direct upload URL for feedback attachments.
type FeedbackPresignRequest struct {
	Filename      string `json:"filename" form:"filename"`
	ContentType   string `json:"content_type" form:"content_type"`
	FileSizeBytes int64  `json:"file_size_bytes,omitempty" form:"file_size_bytes"`
}

// FeedbackPresignResponse carries the temporary pre-signed direct upload coordinates.
type FeedbackPresignResponse struct {
	UploadURL     string `json:"upload_url"`
	ObjectKey     string `json:"object_key"`
	DownloadURL   string `json:"download_url,omitempty"`
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	FileSizeBytes int64  `json:"file_size_bytes,omitempty"`
}
