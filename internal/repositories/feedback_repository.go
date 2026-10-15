package repositories

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateFeedback persists a new user feedback entity into the database.
func (r *Repositories) CreateFeedback(ctx context.Context, feedback *models.Feedback) (*models.Feedback, error) {
	if err := r.db.WithContext(ctx).Create(feedback).Error; err != nil {
		return nil, err
	}
	return feedback, nil
}

// FindFeedbackByIdempotencyKey retrieves feedback by its unique idempotency key.
// Returns nil, nil if no matching feedback is found.
func (r *Repositories) FindFeedbackByIdempotencyKey(ctx context.Context, idempotencyKey string) (*models.Feedback, error) {
	if idempotencyKey == "" {
		return nil, nil
	}

	var feedback models.Feedback
	err := r.db.WithContext(ctx).Where("idempotency_key = ?", idempotencyKey).First(&feedback).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &feedback, nil
}
