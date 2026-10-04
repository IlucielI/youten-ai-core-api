package repositories

import (
	"context"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

// CreateAuditLog records a user/guest sensitive operation.
func (r *Repositories) CreateAuditLog(ctx context.Context, log *models.AuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ListAuditLogsByActor fetches audit logs filtered by actor type and optional user ID.
func (r *Repositories) ListAuditLogsByActor(ctx context.Context, actorType string, actorUserID *uuid.UUID, limit int, offset int) ([]models.AuditLog, int64, error) {
	var logs []models.AuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AuditLog{}).Where("actor_type = ?", actorType)
	if actorUserID != nil {
		query = query.Where("actor_user_id = ?", *actorUserID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// ListAuditLogsByEntity fetches audit logs filtered by entity type and ID.
func (r *Repositories) ListAuditLogsByEntity(ctx context.Context, entityType string, entityID uuid.UUID, limit int, offset int) ([]models.AuditLog, int64, error) {
	var logs []models.AuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AuditLog{}).Where("entity_type = ? AND entity_id = ?", entityType, entityID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// ListRecentAuditLogs fetches latest audit logs across all activities.
func (r *Repositories) ListRecentAuditLogs(ctx context.Context, limit int, offset int) ([]models.AuditLog, int64, error) {
	var logs []models.AuditLog
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AuditLog{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
