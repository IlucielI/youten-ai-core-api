package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateRecording inserts a new recording session into the database.
func (r *Repositories) CreateRecording(ctx context.Context, recording *models.Recording) error {
	return r.db.WithContext(ctx).Create(recording).Error
}

// FindRecordingByID retrieves a recording session by its UUID.
func (r *Repositories) FindRecordingByID(ctx context.Context, id uuid.UUID) (*models.Recording, error) {
	var rec models.Recording
	if err := r.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// FindRecordingByOwnershipToken retrieves a recording matching both ID and secret ownership token.
func (r *Repositories) FindRecordingByOwnershipToken(ctx context.Context, id uuid.UUID, token string) (*models.Recording, error) {
	var rec models.Recording
	if err := r.db.WithContext(ctx).Where("id = ? AND ownership_token = ?", id, token).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// FindRecordingByShareToken retrieves a recording by public share token if sharing is enabled.
func (r *Repositories) FindRecordingByShareToken(ctx context.Context, token string) (*models.Recording, error) {
	var rec models.Recording
	if err := r.db.WithContext(ctx).Where("share_token = ? AND is_share_enabled = TRUE", token).First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateRecording saves full changes to an existing recording.
func (r *Repositories) UpdateRecording(ctx context.Context, recording *models.Recording) error {
	return r.db.WithContext(ctx).Save(recording).Error
}

// UpdateRecordingStatus updates processing state, status, error code, and message.
func (r *Repositories) UpdateRecordingStatus(ctx context.Context, id uuid.UUID, status string, errCode, errMsg *string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if errCode != nil {
		updates["error_code"] = *errCode
	}
	if errMsg != nil {
		updates["error_message"] = *errMsg
	}
	res := r.db.WithContext(ctx).Model(&models.Recording{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ClaimRecordingToUser binds a guest recording session to a registered user account after verifying ownership token.
func (r *Repositories) ClaimRecordingToUser(ctx context.Context, id uuid.UUID, ownershipToken string, userID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&models.Recording{}).
		Where("id = ? AND ownership_token = ? AND is_guest = TRUE", id, ownershipToken).
		Updates(map[string]interface{}{
			"user_id":  userID,
			"is_guest": false,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteRecording performs a soft-delete on a recording.
func (r *Repositories) DeleteRecording(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.Recording{}, "id = ?", id).Error
}

// ListRecordingsByUserID retrieves paginated recordings owned by a user.
func (r *Repositories) ListRecordingsByUserID(ctx context.Context, userID uuid.UUID, limit int, offset int) ([]models.Recording, int64, error) {
	var list []models.Recording
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Recording{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// UpdateRecordingAudioURL sets the audio_url and duration_seconds of a recording.
func (r *Repositories) UpdateRecordingAudioURL(ctx context.Context, id uuid.UUID, audioURL string, durationSeconds float64) error {
	updates := map[string]interface{}{
		"audio_url": audioURL,
	}
	if durationSeconds > 0 {
		updates["duration_seconds"] = durationSeconds
	}
	res := r.db.WithContext(ctx).Model(&models.Recording{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateRecordingAnalytics stores the computed speaker and participation analytics data.
func (r *Repositories) UpdateRecordingAnalytics(ctx context.Context, id uuid.UUID, analytics models.JSONMap) error {
	res := r.db.WithContext(ctx).Model(&models.Recording{}).Where("id = ?", id).Update("analytics_data", analytics)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateRecordingDurationAndLanguage updates the detected language and duration from STT.
func (r *Repositories) UpdateRecordingDurationAndLanguage(ctx context.Context, id uuid.UUID, durationSeconds float64, detectedLanguage string) error {
	updates := map[string]interface{}{}
	if durationSeconds > 0 {
		updates["duration_seconds"] = durationSeconds
	}
	if detectedLanguage != "" {
		updates["detected_language"] = detectedLanguage
	}
	if len(updates) == 0 {
		return nil
	}
	res := r.db.WithContext(ctx).Model(&models.Recording{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

