package dtos

import (
	"encoding/json"
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

// GetRecordingDetailQuery captures optional client query parameters when requesting recording detail.
type GetRecordingDetailQuery struct {
	Token string `form:"token" json:"token"`
}

// TranscriptSegmentDTO represents a diarized speech segment in the recording detail.
type TranscriptSegmentDTO struct {
	ID            string          `json:"id"`
	SpeakerLabel  string          `json:"speaker_label"`
	SpeakerName   string          `json:"speaker_name"`
	StartTime     float64         `json:"start_time"`
	EndTime       float64         `json:"end_time"`
	Text          string          `json:"text"`
	WordsData     json.RawMessage `json:"words_data,omitempty"`
	SequenceOrder int             `json:"sequence_order"`
}

// SummaryDTO represents structured analytical meeting intelligence in the recording detail.
type SummaryDTO struct {
	ID               string                 `json:"id"`
	TemplateCategory string                 `json:"template_category"`
	CustomAngle      *string                `json:"custom_angle,omitempty"`
	Version          int                    `json:"version"`
	IsActive         bool                   `json:"is_active"`
	StructuredData   map[string]interface{} `json:"structured_data"`
	MarkdownContent  string                 `json:"markdown_content"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// ChapterDTO represents a chronological topic boundary in the recording detail.
type ChapterDTO struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	StartTime     float64   `json:"start_time"`
	EndTime       float64   `json:"end_time"`
	Summary       string    `json:"summary"`
	SequenceOrder int       `json:"sequence_order"`
	CreatedAt     time.Time `json:"created_at"`
}

// HighlightDTO represents a key moment bookmark in the recording detail.
type HighlightDTO struct {
	ID        string    `json:"id"`
	StartTime float64   `json:"start_time"`
	EndTime   float64   `json:"end_time"`
	Title     *string   `json:"title,omitempty"`
	Note      *string   `json:"note,omitempty"`
	Source    string    `json:"source"`
	ClipURL   *string   `json:"clip_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// RecordingDetailResponse represents the comprehensive recording details payload.
type RecordingDetailResponse struct {
	ID               string                 `json:"id"`
	UserID           *string                `json:"user_id,omitempty"`
	Title            string                 `json:"title"`
	OriginalFilename string                 `json:"original_filename"`
	FileSizeBytes    int64                  `json:"file_size_bytes"`
	DurationSeconds  float64                `json:"duration_seconds"`
	AudioURL         *string                `json:"audio_url,omitempty"`
	PlaybackURL      *string                `json:"playback_url,omitempty"`
	SourceType       string                 `json:"source_type"`
	Status           string                 `json:"status"`
	ErrorMessage     *string                `json:"error_message,omitempty"`
	ErrorCode        *string                `json:"error_code,omitempty"`
	SelectedTemplate string                 `json:"selected_template"`
	DetectedLanguage *string                `json:"detected_language,omitempty"`
	OutputLanguage   string                 `json:"output_language"`
	IsGuest          bool                   `json:"is_guest"`
	ConsentGiven     bool                   `json:"consent_given"`
	ConsentVersion   string                 `json:"consent_version"`
	ExpiresAt        *time.Time             `json:"expires_at,omitempty"`
	AnalyticsData    map[string]interface{} `json:"analytics_data,omitempty"`
	Segments         []TranscriptSegmentDTO `json:"segments"`
	ActiveSummary    *SummaryDTO            `json:"active_summary,omitempty"`
	Chapters         []ChapterDTO           `json:"chapters"`
	Highlights       []HighlightDTO         `json:"highlights"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}
