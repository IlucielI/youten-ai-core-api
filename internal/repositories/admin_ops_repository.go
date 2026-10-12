package repositories

import (
	"context"

	"code-base-golang/internal/models"
)

// SystemOverviewStats contains high-level database aggregates for telemetry.
type SystemOverviewStats struct {
	TotalUsers           int64
	ActiveUsers          int64
	TotalRecordings      int64
	CompletedRecordings  int64
	FailedRecordings     int64
	TotalStorageBytes    int64
	TotalDurationSeconds float64
}

type systemOverviewStorageDuration struct {
	TotalStorageBytes    int64   `gorm:"column:total_storage"`
	TotalDurationSeconds float64 `gorm:"column:total_duration"`
}

// GetSystemOverviewStats aggregates system telemetry counts across users and recordings.
func (r *Repositories) GetSystemOverviewStats(ctx context.Context) (*SystemOverviewStats, error) {
	var stats SystemOverviewStats

	if err := r.db.WithContext(ctx).Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&models.User{}).Where("status = ?", models.UserStatusActive).Count(&stats.ActiveUsers).Error; err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Count(&stats.TotalRecordings).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Where("status = ?", models.RecordingStatusCompleted).Count(&stats.CompletedRecordings).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Where("status = ?", models.RecordingStatusFailed).Count(&stats.FailedRecordings).Error; err != nil {
		return nil, err
	}

	var sumRes systemOverviewStorageDuration
	err := r.db.WithContext(ctx).Model(&models.Recording{}).
		Select("COALESCE(SUM(file_size_bytes), 0) as total_storage, COALESCE(SUM(duration_seconds), 0) as total_duration").
		Scan(&sumRes).Error
	if err != nil {
		return nil, err
	}

	stats.TotalStorageBytes = sumRes.TotalStorageBytes
	stats.TotalDurationSeconds = sumRes.TotalDurationSeconds

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
