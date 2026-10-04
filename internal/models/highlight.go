package models

import (
	"time"

	"github.com/google/uuid"
)

// Highlight represents a key audio snippet or quote bookmark for instant playback.
type Highlight struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID uuid.UUID  `gorm:"type:uuid;not null;index:idx_highlights_recording_id" json:"recording_id"`
	Recording   *Recording `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	StartTime   float64    `gorm:"not null" json:"start_time"`
	EndTime     float64    `gorm:"not null" json:"end_time"`
	Title       *string    `gorm:"type:varchar(255)" json:"title,omitempty"`
	Note        *string    `gorm:"type:text" json:"note,omitempty"`
	Source      string     `gorm:"type:varchar(50);not null;default:'manual'" json:"source"`
	ClipURL     *string    `gorm:"type:text" json:"clip_url,omitempty"`
	CreatedAt   time.Time  `gorm:"not null;default:now()" json:"created_at"`
}

func (Highlight) TableName() string {
	return "highlights"
}
