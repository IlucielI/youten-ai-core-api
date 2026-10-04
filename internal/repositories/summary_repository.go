package repositories

import (
	"context"

	"github.com/google/uuid"

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
		Where("recording_id = ? AND version = ?", recordingID, version).
		First(&summary).Error
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

// ListSummaryVersions returns all summary versions for a recording.
func (r *Repositories) ListSummaryVersions(ctx context.Context, recordingID uuid.UUID) ([]models.Summary, error) {
	var list []models.Summary
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("version DESC").
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
