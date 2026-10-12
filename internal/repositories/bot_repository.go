package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"

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

// FindBotSessionByExternalID retrieves a bot session by its provider-assigned external session identifier.
func (r *Repositories) FindBotSessionByExternalID(ctx context.Context, externalID string) (*models.BotSession, error) {
	var session models.BotSession
	if err := r.db.WithContext(ctx).First(&session, "external_session_id = ?", externalID).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateBotSessionStatus updates lifecycle status and optional timestamps/errors for a bot session.
func (r *Repositories) UpdateBotSessionStatus(ctx context.Context, id uuid.UUID, status string, errMsg *string, startedAt, endedAt *time.Time) error {
	updates := map[string]any{"status": status}
	setIfNotNull(updates, "error_message", errMsg)
	setIfNotNull(updates, "started_at", startedAt)
	setIfNotNull(updates, "ended_at", endedAt)

	res := r.db.WithContext(ctx).Model(&models.BotSession{}).Where("id = ?", id).Updates(updates)
	return r.ensureRowUpdatedOrExists(ctx, res, &models.BotSession{}, id)
}

// UpdateBotSessionExternalID sets the external session or connection identifier returned by a bot provider.
func (r *Repositories) UpdateBotSessionExternalID(ctx context.Context, id uuid.UUID, externalID string) error {
	res := r.db.WithContext(ctx).Model(&models.BotSession{}).Where("id = ?", id).Update("external_session_id", externalID)
	return r.ensureRowUpdatedOrExists(ctx, res, &models.BotSession{}, id)
}

