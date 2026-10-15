package models

import (
	"time"

	"github.com/google/uuid"
)

// Feedback represents a user feedback, bug report, feature request, or suggestion.
type Feedback struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	UserID         *uuid.UUID `gorm:"type:uuid;index:idx_feedbacks_user_id" json:"user_id,omitempty"`
	User           *User      `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
	UserName       *string    `gorm:"type:varchar(150)" json:"user_name,omitempty"`
	UserEmail      *string    `gorm:"type:varchar(255)" json:"user_email,omitempty"`
	Title          string     `gorm:"type:varchar(255);not null" json:"title"`
	Category       string     `gorm:"type:varchar(50);not null;default:'general';index:idx_feedbacks_category" json:"category"`
	Rating         int        `gorm:"type:int;not null;default:5" json:"rating"`
	Message        string     `gorm:"type:text;not null" json:"message"`
	Status         string     `gorm:"type:varchar(50);not null;default:'new'" json:"status"`
	ClientIP       *string    `gorm:"type:varchar(45)" json:"client_ip,omitempty"`
	UserAgent      *string    `gorm:"type:text" json:"user_agent,omitempty"`
	IdempotencyKey *string    `gorm:"type:varchar(100);uniqueIndex:uidx_feedbacks_idempotency_key" json:"idempotency_key,omitempty"`
	Metadata       JSONMap    `gorm:"type:jsonb;default:'{}'" json:"metadata,omitempty"`
	CreatedAt      time.Time  `gorm:"not null;default:now();index:idx_feedbacks_created_at,sort:desc" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null;default:now()" json:"updated_at"`
}

func (Feedback) TableName() string {
	return "feedbacks"
}
