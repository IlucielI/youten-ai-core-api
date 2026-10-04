package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// CreateInlineComment creates a timestamped comment or threaded reply.
func (r *Repositories) CreateInlineComment(ctx context.Context, comment *models.InlineComment) error {
	return r.db.WithContext(ctx).Create(comment).Error
}

// ListInlineCommentsByRecordingID retrieves top-level comments with preloaded threaded replies.
func (r *Repositories) ListInlineCommentsByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.InlineComment, error) {
	var comments []models.InlineComment
	err := r.db.WithContext(ctx).
		Preload("Replies").
		Where("recording_id = ? AND parent_id IS NULL", recordingID).
		Order("timestamp_sec ASC, created_at ASC").
		Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// FindInlineCommentByID retrieves a comment by its UUID with threaded replies preloaded.
func (r *Repositories) FindInlineCommentByID(ctx context.Context, id uuid.UUID) (*models.InlineComment, error) {
	var comment models.InlineComment
	if err := r.db.WithContext(ctx).Preload("Replies").First(&comment, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &comment, nil
}

// DeleteInlineComment deletes a comment and cascades to child replies.
func (r *Repositories) DeleteInlineComment(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.InlineComment{}, "id = ?", id).Error
}
