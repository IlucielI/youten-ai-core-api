package dtos

import "github.com/google/uuid"

// TemplateItem represents a prompt template returned for client applications.
type TemplateItem struct {
	ID          uuid.UUID `json:"id"`
	CategoryKey string    `json:"category_key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
}

// TemplateListResponse encapsulates the list of active prompt templates.
type TemplateListResponse struct {
	Items []TemplateItem `json:"items"`
}
