package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// TranscriptSegment represents a speech diarization segment with word-level karaoke timing.
type TranscriptSegment struct {
	ID            uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID   uuid.UUID       `gorm:"type:uuid;not null;index:idx_segments_recording_seq,priority:1" json:"recording_id"`
	Recording     *Recording      `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	SpeakerLabel  string          `gorm:"type:varchar(50);not null;default:'Speaker 0'" json:"speaker_label"`
	SpeakerName   string          `gorm:"type:varchar(100);not null;default:'Speaker 0'" json:"speaker_name"`
	StartTime     float64         `gorm:"not null;index:idx_segments_start_time" json:"start_time"`
	EndTime       float64         `gorm:"not null" json:"end_time"`
	Text          string          `gorm:"type:text;not null" json:"text"`
	WordsData     json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"words_data"`
	SequenceOrder int             `gorm:"not null;default:0;index:idx_segments_recording_seq,priority:2" json:"sequence_order"`
	CreatedAt     time.Time       `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"not null;default:now()" json:"updated_at"`
}

func (TranscriptSegment) TableName() string {
	return "transcript_segments"
}
