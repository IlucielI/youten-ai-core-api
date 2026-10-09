package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateBotSession inserts a new bot session record into the database.
func (r *Repositories) CreateBotSession(ctx context.Context, session *models.BotSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// FindBotSessionByID retrieves a bot session by its primary UUID.
func (r *Repositories) FindBotSessionByID(ctx context.Context, id uuid.UUID) (*models.BotSession, error) {
	var session models.BotSession
	if err := r.db.WithContext(ctx).First(&session, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// FindBotSessionByRecordingID retrieves a bot session by its associated recording ID.
func (r *Repositories) FindBotSessionByRecordingID(ctx context.Context, recordingID uuid.UUID) (*models.BotSession, error) {
	var session models.BotSession
	if err := r.db.WithContext(ctx).First(&session, "recording_id = ?", recordingID).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateBotSessionStatus updates lifecycle status and optional timestamps/errors for a bot session.
func (r *Repositories) UpdateBotSessionStatus(ctx context.Context, id uuid.UUID, status string, errMsg *string, startedAt, endedAt *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if errMsg != nil {
		updates["error_message"] = *errMsg
	}
	if startedAt != nil {
		updates["started_at"] = *startedAt
	}
	if endedAt != nil {
		updates["ended_at"] = *endedAt
	}

	res := r.db.WithContext(ctx).Model(&models.BotSession{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateBotSessionExternalID sets the external session or connection identifier returned by a bot provider.
func (r *Repositories) UpdateBotSessionExternalID(ctx context.Context, id uuid.UUID, externalID string) error {
	res := r.db.WithContext(ctx).Model(&models.BotSession{}).Where("id = ?", id).Update("external_session_id", externalID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

