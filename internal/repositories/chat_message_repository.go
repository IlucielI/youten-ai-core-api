package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// SaveChatMessage persists an AI conversational turn or user question with citations.
func (r *Repositories) SaveChatMessage(ctx context.Context, msg *models.ChatMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// ListChatMessagesByRecordingID retrieves the conversation history for a recording session.
func (r *Repositories) ListChatMessagesByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.ChatMessage, error) {
	var messages []models.ChatMessage
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("created_at ASC").
		Find(&messages).Error
	if err != nil {
		return nil, err
	}
	return messages, nil
}
