package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// UserRole represents a customer/user tier role (Free Member, Pro Member, etc.).
type UserRole struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Name        string          `gorm:"type:varchar(100);not null;uniqueIndex" json:"name"`
	Code        string          `gorm:"type:varchar(50);not null;uniqueIndex" json:"code"`
	Description string          `gorm:"type:text" json:"description"`
	Permissions json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"permissions"`
	DailyQuota  int             `gorm:"not null;default:5" json:"daily_quota"`
	IsDefault   bool            `gorm:"not null;default:false" json:"is_default"`
	CreatedAt   time.Time       `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time       `gorm:"not null;default:now()" json:"updated_at"`
}

// TableName overrides the default table name for GORM.
func (UserRole) TableName() string {
	return "user_roles"
}

// PermissionsList decodes raw JSONB permissions into a typed string slice.
func (r *UserRole) PermissionsList() []string {
	if r == nil || len(r.Permissions) == 0 {
		return nil
	}
	var perms []string
	if err := json.Unmarshal(r.Permissions, &perms); err != nil {
		return nil
	}
	return perms
}
