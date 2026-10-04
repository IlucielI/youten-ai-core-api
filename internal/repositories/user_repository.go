package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateUser persists a new user into the database.
func (r *Repositories) CreateUser(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// FindUserByID retrieves a user by their UUID.
func (r *Repositories) FindUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindUserByEmail retrieves a user by their email address.
func (r *Repositories) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("email = ? AND deleted_at IS NULL", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUser saves changes to an existing user.
func (r *Repositories) UpdateUser(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// UpdateUserDailyQuotaOverride sets the custom daily transcription quota for a user.
func (r *Repositories) UpdateUserDailyQuotaOverride(ctx context.Context, id uuid.UUID, quota *int) error {
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Update("daily_quota_override", quota)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
