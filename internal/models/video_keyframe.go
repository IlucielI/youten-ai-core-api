package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// VideoKeyframe represents a captured visual state (slide, screen share, active speaker, demo) from a video recording.
type VideoKeyframe struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID      uuid.UUID       `gorm:"type:uuid;not null;index:idx_video_keyframes_recording_time,priority:1;index:idx_video_keyframes_recording_seq,priority:1" json:"recording_id"`
	Recording        *Recording      `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	TimestampSeconds float64         `gorm:"not null;index:idx_video_keyframes_recording_time,priority:2" json:"timestamp_seconds"`
	S3ImageURL       string          `gorm:"type:text;not null" json:"s3_image_url"`
	CueType          string          `gorm:"type:varchar(50);not null;default:'SLIDE';index:idx_video_keyframes_cue_type" json:"cue_type"`
	Title            string          `gorm:"type:varchar(255)" json:"title"`
	ExtractedText    string          `gorm:"type:text" json:"extracted_text"`
	VisualSummary    string          `gorm:"type:text" json:"visual_summary"`
	Metadata         json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	Confidence       float64         `gorm:"not null;default:1.0" json:"confidence"`
	SequenceOrder    int             `gorm:"not null;default:0;index:idx_video_keyframes_recording_seq,priority:2" json:"sequence_order"`
	CreatedAt        time.Time       `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt        time.Time       `gorm:"not null;default:now()" json:"updated_at"`
}

func (VideoKeyframe) TableName() string {
	return "video_keyframes"
}

