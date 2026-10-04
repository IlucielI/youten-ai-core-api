package models

import (
	"time"

	"github.com/google/uuid"
)

// Chapter represents an automatically detected topic boundary and summary for chronological navigation.
type Chapter struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID   uuid.UUID  `gorm:"type:uuid;not null;index:idx_chapters_recording_seq,priority:1" json:"recording_id"`
	Recording     *Recording `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	Title         string     `gorm:"type:varchar(255);not null" json:"title"`
	StartTime     float64    `gorm:"not null" json:"start_time"`
	EndTime       float64    `gorm:"not null" json:"end_time"`
	Summary       string     `gorm:"type:text;not null;default:''" json:"summary"`
	SequenceOrder int        `gorm:"not null;default:0;index:idx_chapters_recording_seq,priority:2" json:"sequence_order"`
	CreatedAt     time.Time  `gorm:"not null;default:now()" json:"created_at"`
}

func (Chapter) TableName() string {
	return "chapters"
}
