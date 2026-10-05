package dtos

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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

// SpeakerAnalytics represents participation metrics for an individual speaker.
type SpeakerAnalytics struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
	WordCount    int     `json:"word_count"`
	SharePercent float64 `json:"share_percent"`
}

// RecordingAnalytics represents aggregated metrics for a processed recording.
type RecordingAnalytics struct {
	TotalDurationSeconds float64            `json:"total_duration_seconds"`
	TotalWords           int                `json:"total_words"`
	Speakers             []SpeakerAnalytics `json:"speakers"`
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

// PaginationMeta represents pagination details in list responses.
type PaginationMeta struct {
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
}

// RecordingListItem represents a concise summary of a recording in the library list.
type RecordingListItem struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	OriginalFilename string    `json:"original_filename"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	DurationSeconds  float64   `json:"duration_seconds"`
	SourceType       string    `json:"source_type"`
	Status           string    `json:"status"`
	SelectedTemplate string    `json:"selected_template"`
	DetectedLanguage *string   `json:"detected_language,omitempty"`
	OutputLanguage   string    `json:"output_language"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RecordingListResponse contains list of items and pagination metadata.
type RecordingListResponse struct {
	Items      []RecordingListItem `json:"items"`
	Pagination PaginationMeta      `json:"pagination"`
}

// RecordingFilterQuery contains query parameters for filtering and paginating recordings.
type RecordingFilterQuery struct {
	Search    string `form:"search" json:"search"`
	Status    string `form:"status" json:"status"`
	Template  string `form:"template" json:"template"`
	Page      int    `form:"page" json:"page"`
	Limit     int    `form:"limit" json:"limit"`
	SortBy    string `form:"sort_by" json:"sort_by"`
	SortOrder string `form:"sort_order" json:"sort_order"`
}

// SetDefaults sanitizes and defaults pagination and sorting values.
func (q *RecordingFilterQuery) SetDefaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = 10
	} else if q.Limit > 100 {
		q.Limit = 100
	}
	q.SortBy = strings.ToLower(strings.TrimSpace(q.SortBy))
	switch q.SortBy {
	case "title", "duration_seconds", "file_size_bytes", "created_at":
		// valid
	default:
		q.SortBy = "created_at"
	}
	q.SortOrder = strings.ToLower(strings.TrimSpace(q.SortOrder))
	if q.SortOrder != "asc" {
		q.SortOrder = "desc"
	}
}

// ClaimRecordingRequest encapsulates the payload required to bind a guest recording to an authenticated user account.
type ClaimRecordingRequest struct {
	OwnershipToken string `json:"ownership_token"`
}

// BulkClaimRequest encapsulates the list of guest ownership tokens to be claimed to the authenticated user account.
type BulkClaimRequest struct {
	Tokens []string `json:"tokens"`
}

// BulkClaimResponse encapsulates the summary result of a bulk claim operation.
type BulkClaimResponse struct {
	ClaimedCount int      `json:"claimed_count"`
	RecordingIDs []string `json:"recording_ids"`
}

// ShareToggleRequest encapsulates payload to enable or disable public sharing of a recording.
type ShareToggleRequest struct {
	IsShareEnabled *bool `json:"is_share_enabled"`
}

// ShareToggleResponse encapsulates the resulting public sharing state and link.
type ShareToggleResponse struct {
	IsShareEnabled bool    `json:"is_share_enabled"`
	ShareToken     *string `json:"share_token,omitempty"`
	ShareURL       *string `json:"share_url,omitempty"`
}

// SharedRecordingResponse encapsulates the public, read-only payload for a shared recording session.
type SharedRecordingResponse struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	DurationSeconds  float64                `json:"duration_seconds"`
	AudioURL         *string                `json:"audio_url,omitempty"`
	PlaybackURL      *string                `json:"playback_url,omitempty"`
	SelectedTemplate string                 `json:"selected_template"`
	DetectedLanguage *string                `json:"detected_language,omitempty"`
	OutputLanguage   string                 `json:"output_language"`
	Segments         []TranscriptSegmentDTO `json:"segments"`
	ActiveSummary    *SummaryDTO            `json:"active_summary,omitempty"`
	Chapters         []ChapterDTO           `json:"chapters"`
	Highlights       []HighlightDTO         `json:"highlights"`
	CreatedAt        time.Time              `json:"created_at"`
}

// RetryRecordingRequest defines optional parameters for pipeline retry.
type RetryRecordingRequest struct {
	OwnershipToken string `json:"ownership_token,omitempty"`
}

// RetryRecordingResponse represents the response when a recording pipeline retry is accepted.
type RetryRecordingResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Stage     string    `json:"stage"`
	Message   string    `json:"message"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateSpeakersRequest holds the mapping of speaker labels to new speaker names.
type UpdateSpeakersRequest struct {
	Speakers       map[string]string `json:"speakers"`
	OwnershipToken string            `json:"ownership_token,omitempty"`
}

// UpdateSpeakersResponse returns the result of the speaker label rename operation.
type UpdateSpeakersResponse struct {
	UpdatedCount int               `json:"updated_count"`
	Speakers     map[string]string `json:"speakers"`
}

// RegenerateSummaryRequest defines payload for regenerating a recording's summary with an optional template category and custom angle.
type RegenerateSummaryRequest struct {
	TemplateCategory string  `json:"template_category,omitempty"`
	CustomAngle      *string `json:"custom_angle,omitempty"`
	OwnershipToken   string  `json:"ownership_token,omitempty"`
}

// SummaryVersionResponse represents a summary version returned after generation or listing.
type SummaryVersionResponse struct {
	ID               string                 `json:"id"`
	Version          int                    `json:"version"`
	TemplateCategory string                 `json:"template_category"`
	CustomAngle      *string                `json:"custom_angle,omitempty"`
	StructuredData   map[string]interface{} `json:"structured_data"`
	MarkdownContent  string                 `json:"markdown_content"`
	IsActive         bool                   `json:"is_active"`
	CreatedAt        time.Time              `json:"created_at"`
}

// CreateCommentRequest represents the payload to create a timestamped inline comment or reply.
type CreateCommentRequest struct {
	TimestampSec   float64    `json:"timestamp_sec"`
	SegmentID      *uuid.UUID `json:"segment_id,omitempty"`
	SelectedText   *string    `json:"selected_text,omitempty"`
	CommentText    string     `json:"comment_text"`
	AuthorName     string     `json:"author_name"`
	ParentID       *uuid.UUID `json:"parent_id,omitempty"`
	OwnershipToken string     `json:"ownership_token,omitempty"`
}

// CommentResponse represents the comment returned after creation or listing.
type CommentResponse struct {
	ID           string            `json:"id"`
	RecordingID  string            `json:"recording_id,omitempty"`
	UserID       *string           `json:"user_id,omitempty"`
	SegmentID    *string           `json:"segment_id,omitempty"`
	TimestampSec float64           `json:"timestamp_sec"`
	SelectedText *string           `json:"selected_text,omitempty"`
	AuthorName   string            `json:"author_name"`
	CommentText  string            `json:"comment_text"`
	ParentID     *string           `json:"parent_id,omitempty"`
	Replies      []CommentResponse `json:"replies,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at,omitempty"`
}
