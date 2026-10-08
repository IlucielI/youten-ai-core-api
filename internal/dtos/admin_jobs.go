package dtos

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/google/uuid"
)

// AdminJobListQuery defines query parameters for filtering and paginating generation jobs.
type AdminJobListQuery struct {
	Page   int    `form:"page"`
	Limit  int    `form:"limit"`
	Status string `form:"status"`
	Search string `form:"search"`
}

// SetDefaults assigns safe default values for pagination fields.
func (q *AdminJobListQuery) SetDefaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 || q.Limit > 100 {
		q.Limit = 20
	}
}

// Validate validates query parameters.
func (q AdminJobListQuery) Validate() error {
	return validation.ValidateStruct(&q,
		validation.Field(&q.Page, validation.Min(1)),
		validation.Field(&q.Limit, validation.Min(1), validation.Max(100)),
	)
}

// AdminJobItem represents a single generation job or recording row in admin listings.
type AdminJobItem struct {
	ID               uuid.UUID `json:"id"`
	Title            string    `json:"title"`
	OriginalFilename string    `json:"original_filename"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	DurationSeconds  float64   `json:"duration_seconds"`
	SourceType       string    `json:"source_type"`
	Status           string    `json:"status"`
	SelectedTemplate string    `json:"selected_template"`
	DetectedLanguage *string   `json:"detected_language,omitempty"`
	OutputLanguage   string    `json:"output_language"`
	ErrorMessage     *string   `json:"error_message,omitempty"`
	ErrorCode        *string   `json:"error_code,omitempty"`
	IsGuest          bool      `json:"is_guest"`
	UserName         *string   `json:"user_name,omitempty"`
	UserEmail        *string   `json:"user_email,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AdminJobListResponse envelopes paginated generation job items.
type AdminJobListResponse struct {
	Items      []AdminJobItem `json:"items"`
	Total      int64          `json:"total"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	TotalPages int            `json:"total_pages"`
}
