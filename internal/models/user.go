package models

// User represents a registered system user.
type User struct {
	BaseModel
	Email              string  `gorm:"type:varchar(255);not null;uniqueIndex:uidx_users_email,where:deleted_at IS NULL" json:"email"`
	PasswordHash       string  `gorm:"type:varchar(255);not null" json:"-"`
	FullName           string  `gorm:"type:varchar(255);not null" json:"full_name"`
	Status             string  `gorm:"type:varchar(50);not null;default:'active';index:idx_users_status" json:"status"`
	DailyQuotaOverride *int    `gorm:"default:null" json:"daily_quota_override,omitempty"`
	EmailVerified      bool    `gorm:"not null;default:false" json:"email_verified"`
}

func (User) TableName() string {
	return "users"
}
