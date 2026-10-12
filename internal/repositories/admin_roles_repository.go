package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// FindRoleByID retrieves an administrative role by its UUID.
func (r *Repositories) FindRoleByID(ctx context.Context, id uuid.UUID) (*models.AdminRole, error) {
	var role models.AdminRole
	if err := r.db.WithContext(ctx).First(&role, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// ListRolesQuery holds filter criteria for listing administrative roles.
type ListRolesQuery struct {
	Search string
	Offset int
	Limit  int
}

// ListRolesPaginated retrieves paginated administrative roles matching search criteria,
// ordering default system roles first.
func (r *Repositories) ListRolesPaginated(ctx context.Context, q ListRolesQuery) ([]models.AdminRole, int64, error) {
	var roles []models.AdminRole
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AdminRole{})
	if q.Search != "" {
		pattern := "%" + q.Search + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", pattern, pattern)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("is_system DESC, name ASC").Limit(q.Limit).Offset(q.Offset).Find(&roles).Error; err != nil {
		return nil, 0, err
	}

	return roles, total, nil
}

// ListRoles retrieves all administrative roles, ordering default system roles first.
func (r *Repositories) ListRoles(ctx context.Context) ([]models.AdminRole, error) {
	var roles []models.AdminRole
	if err := r.db.WithContext(ctx).Order("is_system DESC, name ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	return roles, nil
}

// FindRoleByName retrieves an administrative role by its unique name.
func (r *Repositories) FindRoleByName(ctx context.Context, name string) (*models.AdminRole, error) {
	var role models.AdminRole
	if err := r.db.WithContext(ctx).First(&role, "name = ?", name).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// CreateRole inserts a new administrative role.
func (r *Repositories) CreateRole(ctx context.Context, role *models.AdminRole) error {
	return r.db.WithContext(ctx).Create(role).Error
}

// UpdateRole saves updates to an existing administrative role.
func (r *Repositories) UpdateRole(ctx context.Context, role *models.AdminRole) error {
	return r.db.WithContext(ctx).Save(role).Error
}
