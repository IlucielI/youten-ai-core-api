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

// DeleteHighlight removes a highlight by its UUID.
func (r *Repositories) DeleteHighlight(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.Highlight{}, "id = ?", id).Error
}
