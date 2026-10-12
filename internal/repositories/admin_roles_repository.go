package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
	"code-base-golang/internal/pkg/strutil"
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

func (q ListRolesQuery) scope(db *gorm.DB) *gorm.DB {
	if pattern := strutil.LikePattern(q.Search); pattern != "" {
		return db.Where("name ILIKE ? OR description ILIKE ?", pattern, pattern)
	}
	return db
}

// ListRolesPaginated retrieves paginated administrative roles matching search criteria,
// ordering default system roles first.
func (r *Repositories) ListRolesPaginated(ctx context.Context, q ListRolesQuery) ([]models.AdminRole, int64, error) {
	var roles []models.AdminRole
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AdminRole{}).Scopes(q.scope)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("is_system DESC, name ASC").Limit(q.Limit).Offset(q.Offset).Find(&roles).Error
	return roles, total, err
}

// ListRoles retrieves all administrative roles, ordering default system roles first.
func (r *Repositories) ListRoles(ctx context.Context) ([]models.AdminRole, error) {
	var roles []models.AdminRole
	err := r.db.WithContext(ctx).Order("is_system DESC, name ASC").Find(&roles).Error
	return roles, err
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
