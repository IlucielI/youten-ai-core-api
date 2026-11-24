package models

import (
	"time"

	"github.com/google/uuid"
)

// AIUsageLog represents an immutable audit log entry for an AI model execution or token consumption.
type AIUsageLog struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           *uuid.UUID `gorm:"type:uuid;index:idx_ai_usage_logs_user_id" json:"user_id,omitempty"`
	WorkspaceID      *uuid.UUID `gorm:"type:uuid;index:idx_ai_usage_logs_workspace_id" json:"workspace_id,omitempty"`
	RecordingID      *uuid.UUID `gorm:"type:uuid;index:idx_ai_usage_logs_recording_id" json:"recording_id,omitempty"`
	ServiceType      string     `gorm:"type:varchar(50);not null;index:idx_ai_usage_logs_service_type" json:"service_type"`
	Provider         string     `gorm:"type:varchar(50);not null" json:"provider"`
	ModelName        string     `gorm:"type:varchar(100);not null" json:"model_name"`
	PromptTokens     int        `gorm:"type:int;not null;default:0" json:"prompt_tokens"`
	CompletionTokens int        `gorm:"type:int;not null;default:0" json:"completion_tokens"`
	TotalTokens      int        `gorm:"type:int;not null;default:0" json:"total_tokens"`
	AudioSeconds     float64    `gorm:"type:numeric(10,2);not null;default:0.00" json:"audio_seconds"`
	LatencyMs        int        `gorm:"type:int;not null;default:0" json:"latency_ms"`
	Metadata         JSONMap    `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt        time.Time  `gorm:"not null;default:now();index:idx_ai_usage_logs_created_at" json:"created_at"`

	// Relations
	User      *User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Workspace *Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	Recording *Recording `gorm:"foreignKey:RecordingID" json:"recording,omitempty"`
}

func (AIUsageLog) TableName() string {
	return "ai_usage_logs"
}
