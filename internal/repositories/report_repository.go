package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"code-base-golang/internal/models"
)

// CreateReport registers a new abuse moderation report.
func (r *Repositories) CreateReport(ctx context.Context, report *models.Report) error {
	return r.db.WithContext(ctx).Create(report).Error
}

// FindReportByID retrieves a moderation report with preloaded recording and handler details.
func (r *Repositories) FindReportByID(ctx context.Context, id uuid.UUID) (*models.Report, error) {
	var report models.Report
	err := r.db.WithContext(ctx).
		Preload("Recording").
		Preload("Handler").
		First(&report, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// ListReportsByStatus retrieves paginated moderation reports, optionally filtered by status.
func (r *Repositories) ListReportsByStatus(ctx context.Context, status string, limit int, offset int) ([]models.Report, int64, error) {
	var list []models.Report
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Report{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Preload("Recording").Preload("Handler").Order("created_at DESC").Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

// UpdateReportResolution marks a report as resolved/dismissed with staff notes.
func (r *Repositories) UpdateReportResolution(ctx context.Context, id uuid.UUID, status string, resolutionNote *string, handledBy uuid.UUID) error {
	updates := map[string]interface{}{
		"status":     status,
		"handled_by": handledBy,
	}
	if resolutionNote != nil {
		updates["resolution_note"] = *resolutionNote
	}
	res := r.db.WithContext(ctx).Model(&models.Report{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
