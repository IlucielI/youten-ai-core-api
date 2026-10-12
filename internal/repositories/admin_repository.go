package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/constants"
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

// FindAdminByEmail retrieves a staff user by their email address with preloaded role.
func (r *Repositories) FindAdminByEmail(ctx context.Context, email string) (*models.AdminUser, error) {
	var admin models.AdminUser
	err := r.db.WithContext(ctx).
		Preload("Role").
		Where("LOWER(email) = LOWER(?) AND deleted_at IS NULL", email).
		First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// UpdateAdminPassword updates the hashed password for an admin user.
func (r *Repositories) UpdateAdminPassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return r.db.WithContext(ctx).
		Model(&models.AdminUser{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"password_hash": passwordHash,
			"updated_at":    time.Now(),
		}).Error
}

// CreateAdminAuthToken stores a new admin authentication or recovery token.
func (r *Repositories) CreateAdminAuthToken(ctx context.Context, token *models.AdminAuthToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// FindAdminAuthTokenByHashAndType retrieves a valid (unrevoked, unexpired) admin token by its hash and type.
func (r *Repositories) FindAdminAuthTokenByHashAndType(ctx context.Context, tokenHash string, tokenType constants.AuthTokenType) (*models.AdminAuthToken, error) {
	var token models.AdminAuthToken
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND type = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash, tokenType, time.Now()).
		First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// RevokeAllAdminAuthTokens revokes all active tokens for an admin by token type.
func (r *Repositories) RevokeAllAdminAuthTokens(ctx context.Context, adminID uuid.UUID, tokenType constants.AuthTokenType) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.AdminAuthToken{}).
		Where("admin_id = ? AND type = ? AND revoked_at IS NULL", adminID, tokenType).
		Update("revoked_at", now).Error
}

// RevokeAdminAuthToken revokes a specific admin token by its ID.
func (r *Repositories) RevokeAdminAuthToken(ctx context.Context, id uuid.UUID) error {
	now := time.Now()
	res := r.db.WithContext(ctx).
		Model(&models.AdminAuthToken{}).
		Where("id = ?", id).
		Update("revoked_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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

// UpdateAdminLastLogin updates the last login timestamp for an admin user.
func (r *Repositories) UpdateAdminLastLogin(ctx context.Context, id uuid.UUID, lastLoginAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.AdminUser{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("last_login_at", lastLoginAt).Error
}

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

// CreateAdminAuditLog logs an internal CMS administrative staff operation.
func (r *Repositories) CreateAdminAuditLog(ctx context.Context, log *models.AdminAuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ListAdminAuditLogs fetches paginated administrative audit logs.
func (r *Repositories) ListAdminAuditLogs(ctx context.Context, limit int, offset int) ([]models.AdminAuditLog, int64, error) {
	return r.ListAdminAuditLogsFiltered(ctx, AdminAuditLogFilter{
		Limit:  limit,
		Offset: offset,
	})
}

// AdminAuditLogFilter specifies filtering and pagination for staff audit logs.
type AdminAuditLogFilter struct {
	Action  string
	Entity  string
	AdminID *uuid.UUID
	Limit   int
	Offset  int
}

// ListAdminAuditLogsFiltered fetches paginated administrative audit logs with optional filters.
func (r *Repositories) ListAdminAuditLogsFiltered(ctx context.Context, f AdminAuditLogFilter) ([]models.AdminAuditLog, int64, error) {
	var logs []models.AdminAuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AdminAuditLog{})
	if f.Action != "" {
		query = query.Where("action = ?", f.Action)
	}
	if f.Entity != "" {
		query = query.Where("entity = ?", f.Entity)
	}
	if f.AdminID != nil && *f.AdminID != uuid.Nil {
		query = query.Where("admin_id = ?", *f.AdminID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Admin").Order("created_at DESC").Limit(f.Limit).Offset(f.Offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// SystemOverviewStats contains high-level database aggregates for telemetry.
type SystemOverviewStats struct {
	TotalUsers           int64
	ActiveUsers          int64
	TotalRecordings      int64
	CompletedRecordings  int64
	FailedRecordings     int64
	TotalStorageBytes    int64
	TotalDurationSeconds float64
}

// GetSystemOverviewStats aggregates system telemetry counts across users and recordings.
func (r *Repositories) GetSystemOverviewStats(ctx context.Context) (*SystemOverviewStats, error) {
	var stats SystemOverviewStats

	if err := r.db.WithContext(ctx).Model(&models.User{}).Count(&stats.TotalUsers).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&models.User{}).Where("status = ?", models.UserStatusActive).Count(&stats.ActiveUsers).Error; err != nil {
		return nil, err
	}

	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Count(&stats.TotalRecordings).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Where("status = ?", models.RecordingStatusCompleted).Count(&stats.CompletedRecordings).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&models.Recording{}).Where("status = ?", models.RecordingStatusFailed).Count(&stats.FailedRecordings).Error; err != nil {
		return nil, err
	}

	type SumResult struct {
		TotalStorageBytes    int64   `gorm:"column:total_storage"`
		TotalDurationSeconds float64 `gorm:"column:total_duration"`
	}
	var sumRes SumResult
	err := r.db.WithContext(ctx).Model(&models.Recording{}).
		Select("COALESCE(SUM(file_size_bytes), 0) as total_storage, COALESCE(SUM(duration_seconds), 0) as total_duration").
		Scan(&sumRes).Error
	if err != nil {
		return nil, err
	}

	stats.TotalStorageBytes = sumRes.TotalStorageBytes
	stats.TotalDurationSeconds = sumRes.TotalDurationSeconds

	return &stats, nil
}

// ListAllRecordingsForAdmin queries paginated generation recordings across all users and statuses.
func (r *Repositories) ListAllRecordingsForAdmin(ctx context.Context, status, search string, limit, offset int) ([]models.Recording, int64, error) {
	query := r.db.WithContext(ctx).Model(&models.Recording{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("title ILIKE ? OR original_filename ILIKE ?", searchPattern, searchPattern)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var recordings []models.Recording
	err := query.
		Preload("User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&recordings).Error
	if err != nil {
		return nil, 0, err
	}

	return recordings, total, nil
}



