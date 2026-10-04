package models

import (
	"time"

	"github.com/google/uuid"
)

// Template represents an analysis prompt and JSON schema template (MOM, 1_ON_1, INTERVIEW, etc.).
type Template struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	CategoryKey  string    `gorm:"type:varchar(100);not null;unique;index:idx_templates_category_key" json:"category_key"`
	Name         string    `gorm:"type:varchar(255);not null" json:"name"`
	Description  string    `gorm:"type:text" json:"description"`
	Prompt       string    `gorm:"type:text;not null" json:"prompt"`
	OutputSchema JSONMap   `gorm:"type:jsonb;not null;default:'{}'" json:"output_schema"`
	Version      int       `gorm:"not null;default:1" json:"version"`
	IsActive     bool      `gorm:"not null;default:true;index:idx_templates_is_active" json:"is_active"`
	CreatedAt    time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (Template) TableName() string {
	return "templates"
}
