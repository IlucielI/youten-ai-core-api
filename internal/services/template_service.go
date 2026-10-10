package services

import (
	"context"

	"code-base-golang/internal/dtos"
)

// ListActiveTemplates retrieves all enabled prompt templates for user selection.
func (s *Service) ListActiveTemplates(ctx context.Context) (*dtos.TemplateListResponse, error) {
	templates, err := s.repo.ListActiveTemplates(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]dtos.TemplateItem, len(templates))
	for i, t := range templates {
		items[i] = dtos.TemplateItem{
			ID:          t.ID,
			CategoryKey: t.CategoryKey,
			Name:        t.Name,
			Description: t.Description,
			IsActive:    t.IsActive,
		}
	}

	return &dtos.TemplateListResponse{Items: items}, nil
}
