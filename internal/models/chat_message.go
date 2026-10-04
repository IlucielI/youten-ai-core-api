package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ChatMessage represents a conversation entry with grounded citations from RAG retrieval.
type ChatMessage struct {
	ID                uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID       uuid.UUID       `gorm:"type:uuid;not null;index:idx_chat_messages_recording,priority:1" json:"recording_id"`
	Recording         *Recording      `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	SenderRole        string          `gorm:"type:varchar(20);not null" json:"sender_role"` // 'user' | 'assistant'
	Content           string          `gorm:"type:text;not null" json:"content"`
	Citations         json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"citations"`
	RetrievedChunkIDs json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"retrieved_chunk_ids"`
	CreatedAt         time.Time       `gorm:"not null;default:now();index:idx_chat_messages_recording,priority:2,sort:asc" json:"created_at"`
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}
