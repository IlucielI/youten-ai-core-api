package repositories

import (
	"context"
	"strings"

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

// ListUsersQuery holds filter criteria for listing users.
type ListUsersQuery struct {
	Search string
	Status models.UserStatus
	Offset int
	Limit  int
}

// ListUsers retrieves paginated registered users matching search query and status filter.
func (r *Repositories) ListUsers(ctx context.Context, q ListUsersQuery) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	// Guardrails: validate and cap pagination bounds to prevent denial-of-service
	if q.Limit <= 0 {
		q.Limit = 20
	} else if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	db := r.db.WithContext(ctx).Model(&models.User{}).Where("deleted_at IS NULL")

	if q.Status != "" {
		db = db.Where("status = ?", q.Status)
	}

	cleanSearch := strings.TrimSpace(q.Search)
	if cleanSearch != "" {
		if len(cleanSearch) > 100 {
			cleanSearch = cleanSearch[:100]
		}
		searchPattern := "%" + cleanSearch + "%"
		db = db.Where("email ILIKE ? OR full_name ILIKE ?", searchPattern, searchPattern)
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := db.Order("created_at DESC").Limit(q.Limit).Offset(q.Offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}


