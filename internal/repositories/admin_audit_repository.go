package repositories

import (
	"context"

	"github.com/google/uuid"

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
