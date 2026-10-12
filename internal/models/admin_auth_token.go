package models

import (
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/constants"
)

// AdminAuthToken represents administrative authentication and verification tokens.
type AdminAuthToken struct {
	ID        uuid.UUID               `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AdminID   uuid.UUID               `gorm:"type:uuid;not null;index:idx_admin_auth_tokens_admin_id" json:"admin_id"`
	Admin     *AdminUser              `gorm:"foreignKey:AdminID;constraint:OnDelete:CASCADE" json:"admin,omitempty"`
	Type      constants.AuthTokenType `gorm:"type:varchar(50);not null;index:idx_admin_auth_tokens_type" json:"type"`
	TokenHash string                  `gorm:"type:varchar(255);not null;index:idx_admin_auth_tokens_hash" json:"-"`
	ExpiresAt time.Time               `gorm:"not null" json:"expires_at"`
	RevokedAt *time.Time              `gorm:"default:null" json:"revoked_at,omitempty"`
	CreatedAt time.Time               `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time               `gorm:"not null;default:now()" json:"updated_at"`
}

func (AdminAuthToken) TableName() string {
	return "admin_auth_tokens"
}
