package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/models"
)

// CreateSummary persists a newly generated structured summary.
func (r *Repositories) CreateSummary(ctx context.Context, summary *models.Summary) error {
	return r.db.WithContext(ctx).Create(summary).Error
}

// FindActiveSummaryByRecordingID retrieves the currently active summary for a recording.
func (r *Repositories) FindActiveSummaryByRecordingID(ctx context.Context, recordingID uuid.UUID) (*models.Summary, error) {
	var summary models.Summary
	err := r.db.WithContext(ctx).
		Where("recording_id = ? AND is_active = TRUE", recordingID).
		Order("version DESC").
		First(&summary).Error
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// FindSummaryByRecordingAndVersion retrieves a specific historical summary version.
func (r *Repositories) FindSummaryByRecordingAndVersion(ctx context.Context, recordingID uuid.UUID, version int) (*models.Summary, error) {
	var summary models.Summary
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Where("version = ?", version).
		First(&summary).Error
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// ListSummaryVersions returns all summary versions for a recording ordered by version ASC.
func (r *Repositories) ListSummaryVersions(ctx context.Context, recordingID uuid.UUID) ([]models.Summary, error) {
	var list []models.Summary
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("version ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// DeactivatePreviousSummaries marks older summaries of a recording as inactive when generating a new version.
func (r *Repositories) DeactivatePreviousSummaries(ctx context.Context, recordingID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&models.Summary{}).
		Where("recording_id = ?", recordingID).
		Update("is_active", false).Error
}

// CountSummaryVersions returns the total number of summary versions for a recording.
func (r *Repositories) CountSummaryVersions(ctx context.Context, recordingID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.Summary{}).
		Where("recording_id = ?", recordingID).
		Count(&count).Error
	return count, err
}

// GetLatestSummaryVersion returns the highest version number for a recording, or 0 if none exist.
func (r *Repositories) GetLatestSummaryVersion(ctx context.Context, recordingID uuid.UUID) (int, error) {
	var maxVer int
	row := r.db.WithContext(ctx).
		Model(&models.Summary{}).
		Where("recording_id = ?", recordingID).
		Select("COALESCE(MAX(version), 0)").
		Row()
	err := row.Scan(&maxVer)
	return maxVer, err
}

// SaveNewSummaryVersion saves a new summary version for a recording within an atomic database transaction,
// enforcing the maximum version cap (5), deactivating previous versions, and setting the new one as active.
func (r *Repositories) SaveNewSummaryVersion(ctx context.Context, summary *models.Summary) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.Summary{}).Where("recording_id = ?", summary.RecordingID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 5 {
			return constants.ErrSummaryVersionLimit
		}

		var maxVer int
		row := tx.Model(&models.Summary{}).
			Where("recording_id = ?", summary.RecordingID).
			Select("COALESCE(MAX(version), 0)").
			Row()
		if err := row.Scan(&maxVer); err != nil {
			return err
		}

		if err := tx.Model(&models.Summary{}).
			Where("recording_id = ?", summary.RecordingID).
			Update("is_active", false).Error; err != nil {
			return err
		}

		summary.Version = maxVer + 1
		summary.IsActive = true
		return tx.Create(summary).Error
	})
}
