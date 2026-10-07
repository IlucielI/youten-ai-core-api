package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AdminRole represents an administrative RBAC role.
type AdminRole struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Name        string          `gorm:"type:varchar(100);not null;unique" json:"name"`
	Description string          `gorm:"type:text" json:"description"`
	Permissions json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"permissions"`
	IsSystem    bool            `gorm:"not null;default:false" json:"is_system"`
	CreatedAt   time.Time       `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time       `gorm:"not null;default:now()" json:"updated_at"`
}

func (AdminRole) TableName() string {
	return "admin_roles"
}

// PermissionsList decodes raw JSONB permissions into a typed string slice.
func (r *AdminRole) PermissionsList() []string {
	if r == nil || len(r.Permissions) == 0 {
		return nil
	}
	var perms []string
	if err := json.Unmarshal(r.Permissions, &perms); err != nil {
		return nil
	}
	return perms
}
