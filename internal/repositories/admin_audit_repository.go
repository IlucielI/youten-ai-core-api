package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

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

// scope applies active filter conditions to the GORM query.
func (f AdminAuditLogFilter) scope(db *gorm.DB) *gorm.DB {
	if f.Action != "" {
		db = db.Where("action = ?", f.Action)
	}
	if f.Entity != "" {
		db = db.Where("entity = ?", f.Entity)
	}
	if f.AdminID != nil && *f.AdminID != uuid.Nil {
		db = db.Where("admin_id = ?", *f.AdminID)
	}
	return db
}

// ListAdminAuditLogsFiltered fetches paginated administrative audit logs with optional filters.
func (r *Repositories) ListAdminAuditLogsFiltered(ctx context.Context, f AdminAuditLogFilter) ([]models.AdminAuditLog, int64, error) {
	var logs []models.AdminAuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AdminAuditLog{}).Scopes(f.scope)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Admin").Order("created_at DESC").Limit(f.Limit).Offset(f.Offset).Find(&logs).Error
	return logs, total, err
}
