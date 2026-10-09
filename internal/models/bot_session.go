package models

import (
	"time"

	"github.com/google/uuid"
)

// BotSession tracks the lifecycle of a meeting voice bot session.
type BotSession struct {
	BaseModel
	RecordingID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_bot_sessions_recording_id" json:"recording_id"`
	Recording         *Recording `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	Provider          string     `gorm:"type:varchar(50);not null;index:idx_bot_sessions_provider" json:"provider"`
	MeetingURL        *string    `gorm:"type:text" json:"meeting_url,omitempty"`
	ExternalSessionID *string    `gorm:"type:varchar(255)" json:"external_session_id,omitempty"`
	ChannelID         *string    `gorm:"type:varchar(100)" json:"channel_id,omitempty"`
	GuildID           *string    `gorm:"type:varchar(100)" json:"guild_id,omitempty"`
	Status            string     `gorm:"type:varchar(50);not null;default:'DISPATCHED';index:idx_bot_sessions_status" json:"status"`
	ErrorMessage      *string    `gorm:"type:text" json:"error_message,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	Metadata          JSONMap    `gorm:"type:jsonb;default:'{}'" json:"metadata,omitempty"`
}

// TableName returns the custom table name for BotSession.
func (BotSession) TableName() string {
	return "bot_sessions"
}
