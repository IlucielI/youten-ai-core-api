package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateNotification saves an in-app milestone notification.
func (r *Repositories) CreateNotification(ctx context.Context, notif *models.Notification) error {
	return r.db.WithContext(ctx).Create(notif).Error
}

// ListNotificationsByUserID retrieves paginated notifications for a user.
func (r *Repositories) ListNotificationsByUserID(ctx context.Context, userID uuid.UUID, limit int, offset int) ([]models.Notification, int64, error) {
	var list []models.Notification
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Notification{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// ListNotificationsByRecordingID retrieves notifications tied to a specific recording.
func (r *Repositories) ListNotificationsByRecordingID(ctx context.Context, recordingID uuid.UUID) ([]models.Notification, error) {
	var list []models.Notification
	err := r.db.WithContext(ctx).
		Where("recording_id = ?", recordingID).
		Order("created_at DESC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// MarkNotificationAsRead marks a notification as read verifying the owner user ID.
func (r *Repositories) MarkNotificationAsRead(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&models.Notification{}).Where("id = ? AND user_id = ?", id, userID).Update("is_read", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
