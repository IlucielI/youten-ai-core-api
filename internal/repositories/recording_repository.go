package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// UpdateRecordingShareSettings updates public sharing status and share token of a recording.
func (r *Repositories) UpdateRecordingShareSettings(ctx context.Context, id uuid.UUID, isShareEnabled bool, shareToken *string) error {
	updates := map[string]interface{}{
		"is_share_enabled": isShareEnabled,
	}
	if shareToken != nil {
		updates["share_token"] = *shareToken
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

// ClaimRecordingsByTokens binds multiple unclaimed guest recording sessions to a registered user account.
func (r *Repositories) ClaimRecordingsByTokens(ctx context.Context, tokens []string, userID uuid.UUID) ([]uuid.UUID, error) {
	if len(tokens) == 0 {
		return []uuid.UUID{}, nil
	}
	var claimedIDs []uuid.UUID
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var recs []models.Recording
		if err := tx.Where("ownership_token IN (?) AND is_guest = TRUE", tokens).
			Find(&recs).Error; err != nil {
			return err
		}
		if len(recs) == 0 {
			return nil
		}
		for _, rec := range recs {
			claimedIDs = append(claimedIDs, rec.ID)
		}
		return tx.Model(&models.Recording{}).
			Where("id IN (?)", claimedIDs).
			Updates(map[string]interface{}{
				"user_id":  userID,
				"is_guest": false,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	if claimedIDs == nil {
		claimedIDs = []uuid.UUID{}
	}
	return claimedIDs, nil
}

// DeleteRecording performs a soft-delete on a recording.
func (r *Repositories) DeleteRecording(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&models.Recording{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// RecordingFilter specifies filtering and pagination criteria for user recordings.
type RecordingFilter struct {
	Search    string
	Status    string
	Template  string
	SortBy    string
	SortOrder string
	Limit     int
	Offset    int
}

// ListRecordingsWithFilter retrieves paginated recordings owned by a user matching criteria.
func (r *Repositories) ListRecordingsWithFilter(ctx context.Context, userID uuid.UUID, filter RecordingFilter) ([]models.Recording, int64, error) {
	var list []models.Recording
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Recording{}).Where("user_id = ?", userID)

	if filter.Search != "" {
		searchPattern := "%" + strings.ToLower(filter.Search) + "%"
		query = query.Where("LOWER(title) LIKE ? OR LOWER(original_filename) LIKE ?", searchPattern, searchPattern)
	}

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if filter.Template != "" {
		query = query.Where("selected_template = ?", filter.Template)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	orderCol := "created_at"
	switch filter.SortBy {
	case "title", "duration_seconds", "file_size_bytes", "created_at":
		orderCol = filter.SortBy
	}

	orderDir := "DESC"
	if strings.ToUpper(filter.SortOrder) == "ASC" {
		orderDir = "ASC"
	}

	orderClause := fmt.Sprintf("%s %s", orderCol, orderDir)

	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	if err := query.Order(orderClause).Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// ListRecordingsByUserID retrieves paginated recordings owned by a user with default ordering.
func (r *Repositories) ListRecordingsByUserID(ctx context.Context, userID uuid.UUID, limit int, offset int) ([]models.Recording, int64, error) {
	return r.ListRecordingsWithFilter(ctx, userID, RecordingFilter{
		Limit:  limit,
		Offset: offset,
	})
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

// CountUserRecordingsToday counts how many recordings the user has created since the beginning of the current UTC day.
func (r *Repositories) CountUserRecordingsToday(ctx context.Context, userID uuid.UUID) (int64, error) {
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Recording{}).
		Where("user_id = ? AND created_at >= ?", userID, startOfDay).
		Count(&count).Error
	return count, err
}

// CountGuestRecordingsToday counts how many recordings a guest IP has created since the beginning of the current UTC day.
func (r *Repositories) CountGuestRecordingsToday(ctx context.Context, guestIP string) (int64, error) {
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Recording{}).
		Where("is_guest = TRUE AND guest_ip = ? AND created_at >= ?", guestIP, startOfDay).
		Count(&count).Error
	return count, err
}

// FindLatestPendingRecording retrieves the most recent pending recording matching filename for a user or guest.
func (r *Repositories) FindLatestPendingRecording(ctx context.Context, filename string, userID *uuid.UUID, guestIP *string) (*models.Recording, error) {
	var rec models.Recording
	query := r.db.WithContext(ctx).Where("original_filename = ? AND status = ?", filename, models.RecordingStatusPending)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	} else if guestIP != nil && *guestIP != "" {
		query = query.Where("is_guest = TRUE AND guest_ip = ?", *guestIP)
	}
	if err := query.Order("created_at DESC").First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

// FindExpiredRecordings retrieves recordings whose expires_at is not null and has passed.
func (r *Repositories) FindExpiredRecordings(ctx context.Context, limit int) ([]models.Recording, error) {
	var list []models.Recording
	err := r.db.WithContext(ctx).
		Where("expires_at IS NOT NULL AND expires_at <= ?", time.Now().UTC()).
		Order("expires_at ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// HardDeleteRecording permanently deletes a recording row, triggering database CASCADE rules.
func (r *Repositories) HardDeleteRecording(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&models.Recording{}, "id = ?", id).Error
}

// ListDLQRecordings queries failed and stuck pipeline recordings with preloaded users.
func (r *Repositories) ListDLQRecordings(ctx context.Context, stuckThreshold time.Duration, limit, offset int) ([]models.Recording, int64, int64, int64, error) {
	thresholdTime := time.Now().UTC().Add(-stuckThreshold)

	stuckCondition := "status IN ('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING') AND updated_at < ?"
	failedCondition := "status = 'FAILED'"

	var failedCount, stuckCount int64

	// Count failed
	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Where(failedCondition).Count(&failedCount).Error; err != nil {
		return nil, 0, 0, 0, err
	}

	// Count stuck
	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Where(stuckCondition, thresholdTime).Count(&stuckCount).Error; err != nil {
		return nil, 0, 0, 0, err
	}

	total := failedCount + stuckCount

	var recordings []models.Recording
	err := r.db.WithContext(ctx).
		Preload("User").
		Where("(status = 'FAILED') OR (status IN ('QUEUED', 'VALIDATING', 'EXTRACTING', 'TRANSCRIBING', 'SUMMARIZING', 'INDEXING') AND updated_at < ?)", thresholdTime).
		Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&recordings).Error
	if err != nil {
		return nil, 0, 0, 0, err
	}

	return recordings, total, failedCount, stuckCount, nil
}

