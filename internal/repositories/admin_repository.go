package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// FindAdminByUsername retrieves a staff user by their username with preloaded role.
func (r *Repositories) FindAdminByUsername(ctx context.Context, username string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("username = ? AND deleted_at IS NULL", username).
		First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// FindAdminByID retrieves a staff user by their UUID with preloaded role.
func (r *Repositories) FindAdminByID(ctx context.Context, id uuid.UUID) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("id = ? AND deleted_at IS NULL", id).
		First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// CreateAdmin creates a new staff user.
func (r *Repositories) CreateAdmin(ctx context.Context, admin *models.AdminUser) error {
	return r.db.WithContext(ctx).Create(admin).Error
}

// FindRoleByID retrieves an administrative role by its UUID.
func (r *Repositories) FindRoleByID(ctx context.Context, id uuid.UUID) (*models.AdminRole, error) {
	var role models.AdminRole
	if err := r.db.WithContext(ctx).First(&role, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// ListRoles retrieves all administrative roles.
func (r *Repositories) ListRoles(ctx context.Context) ([]models.AdminRole, error) {
	var roles []models.AdminRole
	if err := r.db.WithContext(ctx).Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// CreateAdminAuditLog logs an internal CMS administrative staff operation.
func (r *Repositories) CreateAdminAuditLog(ctx context.Context, log *models.AdminAuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ListAdminAuditLogs fetches paginated administrative audit logs.
func (r *Repositories) ListAdminAuditLogs(ctx context.Context, limit int, offset int) ([]models.AdminAuditLog, int64, error) {
	var logs []models.AdminAuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AdminAuditLog{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Admin").Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
