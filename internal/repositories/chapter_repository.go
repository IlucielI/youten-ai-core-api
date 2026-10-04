package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// SaveChapters batch-inserts timestamped chapters.
func (r *Repositories) SaveChapters(ctx context.Context, chapters []models.Chapter) error {
	if len(chapters) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&chapters).Error
}

// ListChaptersByRecordingID retrieves chapters in sequential order.
func (r *Repositories) ListChaptersByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.Chapter, error) {
	var chapters []models.Chapter
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("sequence_order ASC, start_time ASC").
		Limit(100).
		Find(&chapters).Error
	if err != nil {
		return nil, err
	}
	return chapters, nil
}

// DeleteChaptersByRecordingID removes existing chapters when regenerating.
func (r *Repositories) DeleteChaptersByRecordingID(ctx context.Context, recordingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("recording_id = ?", recordingID).Delete(&models.Chapter{}).Error
}
