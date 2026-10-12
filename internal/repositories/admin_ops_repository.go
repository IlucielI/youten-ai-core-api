package repositories

import (
	"context"

	"code-base-golang/internal/models"
)

// SystemOverviewStats contains high-level database aggregates for telemetry.
type SystemOverviewStats struct {
	TotalUsers           int64   `gorm:"column:total_users"`
	ActiveUsers          int64   `gorm:"column:active_users"`
	TotalRecordings      int64   `gorm:"column:total_recordings"`
	CompletedRecordings  int64   `gorm:"column:completed_recordings"`
	FailedRecordings     int64   `gorm:"column:failed_recordings"`
	TotalStorageBytes    int64   `gorm:"column:total_storage_bytes"`
	TotalDurationSeconds float64 `gorm:"column:total_duration_seconds"`
}

// GetSystemOverviewStats aggregates system telemetry counts across users and recordings.
func (r *Repositories) GetSystemOverviewStats(ctx context.Context) (*SystemOverviewStats, error) {
	var stats SystemOverviewStats

	if err := r.db.WithContext(ctx).Model(&models.User{}).
		Select("COUNT(*), COUNT(CASE WHEN status = ? THEN 1 END)", models.UserStatusActive).
		Row().Scan(&stats.TotalUsers, &stats.ActiveUsers); err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Model(&models.Recording{}).
		Select(`
			COUNT(*),
			COUNT(CASE WHEN status = ? THEN 1 END),
			COUNT(CASE WHEN status = ? THEN 1 END),
			COALESCE(SUM(file_size_bytes), 0),
			COALESCE(SUM(duration_seconds), 0)
		`, models.RecordingStatusCompleted, models.RecordingStatusFailed).
		Row().Scan(
		&stats.TotalRecordings,
		&stats.CompletedRecordings,
		&stats.FailedRecordings,
		&stats.TotalStorageBytes,
		&stats.TotalDurationSeconds,
	); err != nil {
		return nil, err
	}

	return &stats, nil
}

// ListAllRecordingsForAdmin queries paginated generation recordings across all users and statuses.
func (r *Repositories) ListAllRecordingsForAdmin(ctx context.Context, status, search string, limit, offset int) ([]models.Recording, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.Recording{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("title ILIKE ? OR original_filename ILIKE ?", searchPattern, searchPattern)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var recordings []models.Recording
	err := query.
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&recordings).Error
	if err != nil {
		return nil, 0, err
	}

	return recordings, total, nil
}
