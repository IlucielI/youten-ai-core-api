package models

import (
	"time"

	"github.com/google/uuid"
)

// Canonical recording lifecycle states
const (
	RecordingStatusPending     = "PENDING"
	RecordingStatusQueued       = "QUEUED"
	RecordingStatusValidating   = "VALIDATING"
	RecordingStatusExtracting   = "EXTRACTING"
	RecordingStatusRecording    = "RECORDING"
	RecordingStatusTranscribing = "TRANSCRIBING"
	RecordingStatusSummarizing  = "SUMMARIZING"
	RecordingStatusIndexing     = "INDEXING"
	RecordingStatusCompleted    = "COMPLETED"
	RecordingStatusFailed       = "FAILED"
)

// Standard pipeline error codes
const (
	ErrCodeAudioCorrupt       = "ERR_AUDIO_CORRUPT"
	ErrCodeExtractionFailed   = "ERR_EXTRACTION_FAILED"
	ErrCodeTranscriptionFail = "ERR_TRANSCRIPTION_FAILED"
	ErrCodeNoSpeechDetected   = "ERR_NO_SPEECH_DETECTED"
	ErrCodeSummarizationFail  = "ERR_SUMMARIZATION_FAILED"
	ErrCodeIndexingFail       = "ERR_INDEXING_FAILED"
	ErrCodeImportFetchFailed    = "ERR_IMPORT_FETCH_FAILED"
	ErrCodeGDriveAccessDenied   = "ERR_GDRIVE_ACCESS_DENIED"
	ErrCodeUnsupportedMediaType = "ERR_UNSUPPORTED_MEDIA_TYPE"
)

// Recording represents a core media file, its processing lifecycle, guest ownership, and sharing configuration.
type Recording struct {
	BaseModel
	UserID           *uuid.UUID `gorm:"type:uuid;index:idx_recordings_user_id" json:"user_id,omitempty"`
	User             *User      `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
	OwnershipToken   string     `gorm:"type:varchar(255);not null;index:idx_recordings_ownership_token" json:"-"`
	ShareToken       *string    `gorm:"type:varchar(255);index:idx_recordings_share_token" json:"share_token,omitempty"`
	IsShareEnabled   bool       `gorm:"not null;default:false" json:"is_share_enabled"`
	Title            string     `gorm:"type:varchar(255);not null" json:"title"`
	OriginalFilename string     `gorm:"type:varchar(255);not null" json:"original_filename"`
	FileSizeBytes    int64      `gorm:"not null;default:0" json:"file_size_bytes"`
	DurationSeconds  float64    `gorm:"not null;default:0" json:"duration_seconds"`
	AudioURL         *string    `gorm:"type:text" json:"audio_url,omitempty"`
	SourceType       string     `gorm:"type:varchar(50);not null;default:'UPLOAD'" json:"source_type"`
	BotProvider      *string    `gorm:"type:varchar(50)" json:"bot_provider,omitempty"`
	Status           string     `gorm:"type:varchar(50);not null;default:'QUEUED';index:idx_recordings_status" json:"status"`
	ErrorMessage     *string    `gorm:"type:text" json:"error_message,omitempty"`
	ErrorCode        *string    `gorm:"type:varchar(100)" json:"error_code,omitempty"`
	SelectedTemplate string     `gorm:"type:varchar(100);not null;default:'GENERAL';index:idx_recordings_template" json:"selected_template"`
	DetectedLanguage *string    `gorm:"type:varchar(50)" json:"detected_language,omitempty"`
	OutputLanguage   string     `gorm:"type:varchar(50);not null;default:'id'" json:"output_language"`
	AnalyticsData    JSONMap    `gorm:"type:jsonb" json:"analytics_data,omitempty"`
	IsGuest          bool       `gorm:"not null" json:"is_guest"`
	GuestIP          *string    `gorm:"type:varchar(100)" json:"guest_ip,omitempty"`
	ConsentGiven     bool       `gorm:"not null;default:false" json:"consent_given"`
	ConsentVersion   string     `gorm:"type:varchar(50);not null;default:'1.0'" json:"consent_version"`
	ConsentAt        *time.Time `json:"consent_at,omitempty"`
	ExpiresAt        *time.Time `gorm:"index:idx_recordings_expires_at" json:"expires_at,omitempty"`
}

func (Recording) TableName() string {
	return "recordings"
}
