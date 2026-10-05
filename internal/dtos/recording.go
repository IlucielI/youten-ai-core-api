package dtos

import (
	"path/filepath"
	"strings"
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

// SupportedMIMETypes enumerates the allowed audio and video MIME formats.
var SupportedMIMETypes = map[string]bool{
	"audio/mpeg":      true,
	"audio/mp3":       true,
	"audio/wav":       true,
	"audio/x-wav":     true,
	"audio/wave":      true,
	"audio/mp4":       true,
	"video/mp4":       true,
	"audio/m4a":       true,
	"audio/x-m4a":     true,
	"audio/webm":      true,
	"video/webm":      true,
	"audio/ogg":       true,
	"video/quicktime": true,
}

// SupportedExtensions enumerates supported file extensions for media type checking.
var SupportedExtensions = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".mp4":  true,
	".m4a":  true,
	".webm": true,
	".ogg":  true,
	".mov":  true,
}

// IsValidMediaMIME checks whether a Content-Type and file extension belong to supported media formats.
func IsValidMediaMIME(contentType, filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if !SupportedExtensions[ext] {
		return false
	}

	cleanMIME := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if cleanMIME == "" || cleanMIME == "application/octet-stream" {
		return true
	}

	return SupportedMIMETypes[cleanMIME]
}

// PresignUploadRequest carries the target media filename for generating a presigned direct upload URL.
type PresignUploadRequest struct {
	Filename    string `form:"filename" json:"filename"`
	ContentType string `form:"content_type" json:"content_type"`
}

// Validate checks request constraints for presign upload.
func (r PresignUploadRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Filename, validation.Required, validation.Length(1, 255)),
	)
}

// PresignUploadResponse carries the temporary pre-signed direct upload coordinates.
type PresignUploadResponse struct {
	UploadURL string `json:"upload_url"`
	ObjectKey string `json:"object_key"`
	Filename  string `json:"filename"`
}

// UploadRecordingRequest carries metadata sent by the client after uploading directly to S3.
type UploadRecordingRequest struct {
	Filename  string `form:"filename" json:"filename"`
	ObjectKey string `form:"object_key" json:"object_key"`
	Title     string `form:"title" json:"title"`
	Template  string `form:"template" json:"template"`
	Language  string `form:"language" json:"language"`
}

// Validate checks request constraints.
func (r UploadRecordingRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Filename, validation.Required, validation.Length(1, 255)),
		validation.Field(&r.Title, validation.Length(0, 255)),
		validation.Field(&r.Template, validation.Length(0, 100)),
		validation.Field(&r.Language, validation.Length(0, 50)),
	)
}

// RecordingUploadResponse represents the response payload returned after successful ingestion.
type RecordingUploadResponse struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	OriginalFilename string    `json:"original_filename"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	Status           string    `json:"status"`
	SelectedTemplate string    `json:"selected_template"`
	OutputLanguage   string    `json:"output_language"`
	IsGuest          bool      `json:"is_guest"`
	OwnershipToken   *string   `json:"ownership_token,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// ImportURLRequest carries target URL and parameters for link import media ingestion.
type ImportURLRequest struct {
	URL      string `form:"url" json:"url"`
	Title    string `form:"title" json:"title"`
	Template string `form:"template" json:"template"`
	Language string `form:"language" json:"language"`
}

// Validate checks request constraints for URL import.
func (r ImportURLRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.URL, validation.Required, is.URL),
		validation.Field(&r.Title, validation.Length(0, 255)),
		validation.Field(&r.Template, validation.Length(0, 100)),
		validation.Field(&r.Language, validation.Length(0, 50)),
	)
}
