package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ClientApp represents an authorized client application (web app, cms, etc.) allowed to connect.
type ClientApp struct {
	ID               uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	ClientID         string          `gorm:"type:varchar(100);not null;uniqueIndex" json:"client_id"`
	ClientSecretHash string          `gorm:"type:varchar(255);not null" json:"-"`
	Name             string          `gorm:"type:varchar(150);not null" json:"name"`
	Description      string          `gorm:"type:text" json:"description"`
	AllowedScopes    json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"allowed_scopes"`
	IsActive         bool            `gorm:"not null;default:true" json:"is_active"`
	CreatedAt        time.Time       `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt        time.Time       `gorm:"not null;default:now()" json:"updated_at"`
}

// TableName overrides the default table name for GORM.
func (ClientApp) TableName() string {
	return "client_apps"
}

// ScopesList decodes raw JSONB allowed_scopes into a typed string slice.
func (c *ClientApp) ScopesList() []string {
	if c == nil || len(c.AllowedScopes) == 0 {
		return nil
	}
	var scopes []string
	if err := json.Unmarshal(c.AllowedScopes, &scopes); err != nil {
		return nil
	}
	return scopes
}
