package repositories

import (
	"context"

	"gorm.io/gorm"

	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/strutil"
)

// AdminRecordingsFilter specifies criteria and pagination for admin job listings.
type AdminRecordingsFilter struct {
	Status string
	Search string
	Limit  int
	Offset int
}

func (f AdminRecordingsFilter) scope(db *gorm.DB) *gorm.DB {
	if f.Status != "" {
		db = db.Where("status = ?", f.Status)
	}
	if pattern := strutil.LikePattern(f.Search); pattern != "" {
		db = db.Where("title ILIKE ? OR original_filename ILIKE ?", pattern, pattern)
	}
	return db
}

// ListAllRecordingsForAdmin queries paginated generation recordings across all users and statuses.
func (r *Repositories) ListAllRecordingsForAdmin(ctx context.Context, f AdminRecordingsFilter) ([]models.Recording, int64, error) {
	var recordings []models.Recording
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Recording{}).Scopes(f.scope)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Preload("User").
		Order("created_at DESC").
		Limit(f.Limit).
		Offset(f.Offset).
		Find(&recordings).Error

	return recordings, total, err
}

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
