package models

import (
	"time"

	"github.com/google/uuid"
)

// BotWaitlist represents an applicant in the meeting voice bot beta waitlist.
type BotWaitlist struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email       string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"email"`
	Platform    string    `gorm:"type:varchar(50);not null;default:'google_meet'" json:"platform"`
	CompanySize string    `gorm:"type:varchar(50);not null;default:'1-10'" json:"company_size"`
	Status      string    `gorm:"type:varchar(50);not null;default:'PENDING'" json:"status"`
	CreatedAt   time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

// TableName returns the custom table name for BotWaitlist.
func (BotWaitlist) TableName() string {
	return "bot_waitlists"
}
