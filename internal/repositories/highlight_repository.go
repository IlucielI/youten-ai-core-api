package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// CreateHighlight inserts a new key moment highlight.
func (r *Repositories) CreateHighlight(ctx context.Context, highlight *models.Highlight) error {
	return r.db.WithContext(ctx).Create(highlight).Error
}

// ListHighlightsByRecordingID retrieves key moment highlights chronologically.
func (r *Repositories) ListHighlightsByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.Highlight, error) {
	var highlights []models.Highlight
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("start_time ASC").
		Find(&highlights).Error
	if err != nil {
		return nil, err
	}
	return highlights, nil
}

// SaveHighlights batch-inserts key moment highlights.
func (r *Repositories) SaveHighlights(ctx context.Context, highlights []models.Highlight) error {
	if len(highlights) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&highlights).Error
}

// DeleteHighlight removes a highlight by its UUID.
func (r *Repositories) DeleteHighlight(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.Highlight{}, "id = ?", id).Error
}

// DeleteHighlightsByRecordingID removes all highlights for a recording.
func (r *Repositories) DeleteHighlightsByRecordingID(ctx context.Context, recordingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("recording_id = ?", recordingID).Delete(&models.Highlight{}).Error
}

