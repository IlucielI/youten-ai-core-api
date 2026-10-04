package models

import (
	"time"

	"github.com/google/uuid"
)

// TranscriptChunk represents an indexed text chunk for grounded RAG semantic vector search.
type TranscriptChunk struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	RecordingID uuid.UUID  `gorm:"type:uuid;not null;index:idx_chunks_recording_index,priority:1" json:"recording_id"`
	Recording   *Recording `gorm:"foreignKey:RecordingID;constraint:OnDelete:CASCADE" json:"recording,omitempty"`
	ChunkIndex  int        `gorm:"not null;default:0;index:idx_chunks_recording_index,priority:2" json:"chunk_index"`
	Content     string     `gorm:"type:text;not null" json:"content"`
	StartTime   float64    `gorm:"not null;default:0" json:"start_time"`
	EndTime     float64    `gorm:"not null;default:0" json:"end_time"`
	Embedding   Vector     `gorm:"type:vector(1024)" json:"embedding,omitempty"`
	CreatedAt   time.Time  `gorm:"not null;default:now()" json:"created_at"`
}

func (TranscriptChunk) TableName() string {
	return "transcript_chunks"
}
