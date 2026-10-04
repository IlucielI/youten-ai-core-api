package models

import (
	"time"

	"github.com/google/uuid"
)

// AdminUser represents a staff member or administrator user.
type AdminUser struct {
	BaseModel
	Username     string     `gorm:"type:varchar(100);not null;unique" json:"username"`
	PasswordHash string     `gorm:"type:varchar(255);not null" json:"-"`
	FullName     string     `gorm:"type:varchar(255);not null" json:"full_name"`
	RoleID       uuid.UUID  `gorm:"type:uuid;not null;index:idx_admin_users_role_id" json:"role_id"`
	Role         *AdminRole `gorm:"foreignKey:RoleID;constraint:OnDelete:RESTRICT" json:"role,omitempty"`
	Status       string     `gorm:"type:varchar(50);not null;default:'active';index:idx_admin_users_status" json:"status"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

func (AdminUser) TableName() string {
	return "admin_users"
}
