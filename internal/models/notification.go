package models

import (
	"time"

	"github.com/google/uuid"
)

// Notification represents an in-app milestone alert for recording progress and completion.
type Notification struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	UserID      *uuid.UUID `gorm:"type:uuid;index:idx_notifications_user_id" json:"user_id,omitempty"`
	User        *User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	RecordingID *uuid.UUID `gorm:"type:uuid;index:idx_notifications_recording_id" json:"recording_id,omitempty"`
	Recording   *Recording `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Message     string     `gorm:"type:text;not null" json:"message"`
	Type        string     `gorm:"type:varchar(50);not null;default:'info'" json:"type"`
	IsRead      bool       `gorm:"not null;default:false" json:"is_read"`
	CreatedAt   time.Time  `gorm:"not null;default:now()" json:"created_at"`
}

func (Notification) TableName() string {
	return "notifications"
}
