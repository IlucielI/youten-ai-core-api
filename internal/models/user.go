package models

import (
	"github.com/google/uuid"
)

// UserStatus represents the domain status of a user account.
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusPending   UserStatus = "pending"
)

// User represents a registered system user.
type User struct {
	BaseModel
	Email              string     `gorm:"type:varchar(255);not null;uniqueIndex:uidx_users_email,where:deleted_at IS NULL" json:"email"`
	PasswordHash       string     `gorm:"type:varchar(255);not null" json:"-"`
	FullName           string     `gorm:"type:varchar(255);not null" json:"full_name"`
	Status             UserStatus `gorm:"type:varchar(50);not null;default:'active';index:idx_users_status" json:"status"`
	RoleID             *uuid.UUID `gorm:"type:uuid;index:idx_users_role_id" json:"role_id,omitempty"`
	Role               *UserRole  `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	DailyQuotaOverride *int       `gorm:"default:null" json:"daily_quota_override,omitempty"`
	EmailVerified      bool       `gorm:"not null;default:false" json:"email_verified"`
}

func (User) TableName() string {
	return "users"
}
