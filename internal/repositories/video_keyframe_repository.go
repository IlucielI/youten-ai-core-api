package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// SaveVideoKeyframes batch-inserts extracted video keyframe records.
func (r *Repositories) SaveVideoKeyframes(ctx context.Context, frames []models.VideoKeyframe) error {
	if len(frames) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&frames).Error
}

// ListVideoKeyframesByRecordingID retrieves all keyframes for a recording in chronological order.
func (r *Repositories) ListVideoKeyframesByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.VideoKeyframe, error) {
	var frames []models.VideoKeyframe
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("sequence_order ASC, timestamp_seconds ASC").
		Find(&frames).Error
	if err != nil {
		return nil, err
	}
	return frames, nil
}

// DeleteVideoKeyframesByRecordingID removes all keyframes associated with a recording.
func (r *Repositories) DeleteVideoKeyframesByRecordingID(ctx context.Context, recordingID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Delete(&models.VideoKeyframe{}).Error
}

