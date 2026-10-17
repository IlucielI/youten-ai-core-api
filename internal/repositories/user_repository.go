package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/strutil"
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

// FindUserByIDWithRole retrieves a user by their UUID with their UserRole preloaded.
func (r *Repositories) FindUserByIDWithRole(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Preload("Role").First(&user, "id = ?", id).Error; err != nil {
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

// UpdateUserRoleID updates the assigned user role for a user.
func (r *Repositories) UpdateUserRoleID(ctx context.Context, id uuid.UUID, roleID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ? AND deleted_at IS NULL", id).Update("role_id", roleID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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

// UpdateUserMaxFileSizeMB sets or clears the custom max file size override for a user in MB.
func (r *Repositories) UpdateUserMaxFileSizeMB(ctx context.Context, id uuid.UUID, maxFileSizeMB *int) error {
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Update("max_file_size_mb", maxFileSizeMB)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUserPassword updates the password hash for a specific user ID.
func (r *Repositories) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Update("password_hash", passwordHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUserFullName updates the display name for a specific user ID.
func (r *Repositories) UpdateUserFullName(ctx context.Context, id uuid.UUID, fullName string) error {
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Update("full_name", fullName)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUserStatus updates account status (e.g. active, suspended) for a user.
func (r *Repositories) UpdateUserStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error {
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ? AND deleted_at IS NULL", id).Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListUsersQuery holds filter criteria for listing users.
type ListUsersQuery struct {
	Search string
	Status models.UserStatus
	Offset int
	Limit  int
}

func (q ListUsersQuery) scope(db *gorm.DB) *gorm.DB {
	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}

	if pattern := strutil.LikePattern(q.Search); pattern != "" {
		db = db.Where("email ILIKE ? OR full_name ILIKE ?", pattern, pattern)
	}

	return db
}

// ListUsers retrieves paginated registered users matching search query and status filter.
func (r *Repositories) ListUsers(ctx context.Context, q ListUsersQuery) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	db := r.db.WithContext(ctx).Model(&models.User{}).
		Where("deleted_at IS NULL").
		Scopes(q.scope)

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := db.Order("created_at DESC").Limit(q.Limit).Offset(q.Offset).Find(&users).Error
	return users, total, err
}

// MarkEmailVerified updates user email_verified to true and sets status to active.
func (r *Repositories) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&models.User{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"email_verified": true,
			"status":         models.UserStatusActive,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
