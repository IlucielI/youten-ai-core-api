package models

import (
	"time"

	"github.com/google/uuid"
)

// AuthToken represents authentication and verification tokens.
type AuthToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_auth_tokens_user_id" json:"user_id"`
	User      *User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	Type      string     `gorm:"type:varchar(50);not null;index:idx_auth_tokens_type" json:"type"`
	TokenHash string     `gorm:"type:varchar(255);not null;index:idx_auth_tokens_hash" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt *time.Time `gorm:"default:null" json:"revoked_at,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

func (AuthToken) TableName() string {
	return "auth_tokens"
}
