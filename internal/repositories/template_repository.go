package repositories

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"code-base-golang/internal/models"
)

// FindTemplateByCategoryKey retrieves an active template by its unique category key (e.g. "MOM", "TECH_REVIEW").
func (r *Repositories) FindTemplateByCategoryKey(ctx context.Context, categoryKey string) (*models.Template, error) {
	var tmpl models.Template
	if err := r.db.WithContext(ctx).Where("category_key = ? AND is_active = TRUE", categoryKey).First(&tmpl).Error; err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// FindAnyTemplateByCategoryKey retrieves a template by category key regardless of active state.
func (r *Repositories) FindAnyTemplateByCategoryKey(ctx context.Context, categoryKey string) (*models.Template, error) {
	var tmpl models.Template
	if err := r.db.WithContext(ctx).First(&tmpl, "category_key = ?", categoryKey).Error; err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// CreateTemplate inserts a new prompt template.
func (r *Repositories) CreateTemplate(ctx context.Context, tmpl *models.Template) error {
	return r.db.WithContext(ctx).Create(tmpl).Error
}

// FindTemplateByID retrieves a template by its UUID.
func (r *Repositories) FindTemplateByID(ctx context.Context, id uuid.UUID) (*models.Template, error) {
	var tmpl models.Template
	if err := r.db.WithContext(ctx).First(&tmpl, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &tmpl, nil
}

// UpdateTemplate saves updates to an existing prompt template.
func (r *Repositories) UpdateTemplate(ctx context.Context, tmpl *models.Template) error {
	return r.db.WithContext(ctx).Save(tmpl).Error
}

// ListActiveTemplates lists all enabled prompt templates ordered by name.
func (r *Repositories) ListActiveTemplates(ctx context.Context) ([]models.Template, error) {
	var list []models.Template
	if err := r.db.WithContext(ctx).Where("is_active = TRUE").Order("name ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListAllTemplates lists all templates (both active and inactive) ordered by category key.
func (r *Repositories) ListAllTemplates(ctx context.Context) ([]models.Template, error) {
	var list []models.Template
	if err := r.db.WithContext(ctx).Order("category_key ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpsertTemplate creates or updates a template by category_key.
func (r *Repositories) UpsertTemplate(ctx context.Context, tmpl *models.Template) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "category_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name",
			"description",
			"prompt",
			"output_schema",
			"version",
			"is_active",
			"updated_at",
		}),
	}).Create(tmpl).Error
}
