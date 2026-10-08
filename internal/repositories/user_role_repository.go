package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// FindUserRoleByID retrieves a user role by its UUID.
func (r *Repositories) FindUserRoleByID(ctx context.Context, id uuid.UUID) (*models.UserRole, error) {
	var role models.UserRole
	if err := r.db.WithContext(ctx).First(&role, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// FindUserRoleByCode retrieves a user role by its unique code (e.g. FREE, PRO).
func (r *Repositories) FindUserRoleByCode(ctx context.Context, code string) (*models.UserRole, error) {
	var role models.UserRole
	if err := r.db.WithContext(ctx).Where("UPPER(code) = UPPER(?)", code).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// FindDefaultUserRole retrieves the default user role assigned to new registrations.
func (r *Repositories) FindDefaultUserRole(ctx context.Context) (*models.UserRole, error) {
	var role models.UserRole
	if err := r.db.WithContext(ctx).Where("is_default = TRUE").First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// ListUserRoles retrieves all available user roles ordered by daily quota ascending.
func (r *Repositories) ListUserRoles(ctx context.Context) ([]models.UserRole, error) {
	var roles []models.UserRole
	if err := r.db.WithContext(ctx).Order("daily_quota ASC, created_at ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// CreateUserRole persists a new customer user role into the database.
func (r *Repositories) CreateUserRole(ctx context.Context, role *models.UserRole) error {
	return r.db.WithContext(ctx).Create(role).Error
}

// UpdateUserRole updates an existing user role's attributes.
func (r *Repositories) UpdateUserRole(ctx context.Context, role *models.UserRole) error {
	return r.db.WithContext(ctx).Save(role).Error
}
